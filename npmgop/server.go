package npmgop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bep/npmgoproxy/internal"

	"golang.org/x/mod/module"
)

const npmjsPrefix = internal.ModPathBase + "/"

var (
	apiList = regexp.MustCompile(`^/(?P<module>.*)/@v/list$`)
	apiInfo = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).info$`)
	apiMod  = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).mod$`)
	apiZip  = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).zip$`)
)

func Start() (*Server, error) {
	l, err := net.Listen("tcp", "localhost:8072")
	if err != nil {
		return nil, err
	}

	httpServer := &http.Server{Addr: ":8072", Handler: &npmGoModProxy{}}
	s := &Server{
		httpServer: httpServer,
	}

	go func() {
		if err := httpServer.Serve(l); err != nil {
			if err != http.ErrServerClosed {
				s.err = err
			}
		}
	}()

	return s, nil
}

type Server struct {
	err        error
	httpServer *http.Server
}

func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}
	return s.err
}

type moduleContext struct {
	NpmPackage       string
	Version          string
	PathMajorVersion string
}

func (ctx moduleContext) String() string {
	return fmt.Sprintf("%s|%s|%s", ctx.NpmPackage, ctx.Version, ctx.PathMajorVersion)
}

type npmGoModProxy struct{}

// $base/$module/@v/$version.info
// Returns JSON-formatted metadata about a specific version of a module.
func (g *npmGoModProxy) Info(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("npmgomodproxy.info", mctx)

	npmv, err := internal.FetchPackageVersion(mctx.NpmPackage, mctx.Version)
	if err != nil {
		g.fail(w, "failed to fetch package version", err)
		return
	}

	g.encodeVersion(w, npmv)

}

func (g *npmGoModProxy) List(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("npmgomodproxy.list", mctx)

	npmpkg, err := internal.FetchPackage(mctx.NpmPackage)
	if err != nil {
		g.fail(w, "failed to fetch package", err)
		return
	}

	var versions []string
	for _, v := range npmpkg.Versions {
		versions = append(versions, v.Version)
	}

	fmt.Fprint(w, strings.Join(versions, "\n"))
}

// $base/$module/@v/$version.mod
// Returns the go.mod file for a specific version of a module. If the module does
// not have a go.mod file at the requested version, a file containing only a
// module statement with the requested module path must be returned. Otherwise,
// the original, unmodified go.mod file must be returned.
func (g *npmGoModProxy) Mod(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("npmgomodproxy.mod", mctx)

	npmv, err := internal.FetchPackageVersion(mctx.NpmPackage, mctx.Version)
	if err != nil {
		g.fail(w, "failed to fetch package version", err)
		return
	}

	depLine := func(dep internal.Dependency) string {
		return fmt.Sprintf("\tgohugo.io/npmjs/%s/v3 %s\n", internal.EscapePackage(dep.Name), "v3.1.1") // TODO1 version range + mahor path?
	}

	var requires string
	if len(npmv.Dependencies) > 0 {
		requires = "require (\n"
		for _, dep := range npmv.Dependencies {
			requires += depLine(dep)
		}
		requires += ")\n"
	}

	gomod := `

module gohugo.io/npmjs/%s

%s

go 1.17
	
	
`

	fmt.Fprintf(w, gomod, path.Join(mctx.NpmPackage, mctx.PathMajorVersion), requires)
}

func (g *npmGoModProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/"+npmjsPrefix) {
		http.NotFound(w, r)
		return
	}

	for _, route := range []struct {
		id      string
		regexp  *regexp.Regexp
		handler func(w http.ResponseWriter, r *http.Request, mctx moduleContext)
	}{
		{"list", apiList, g.List},
		{"info", apiInfo, g.Info},
		{"npmgomodproxy", apiMod, g.Mod},
		{"zip", apiZip, g.Zip},
	} {
		if m := route.regexp.FindStringSubmatch(r.URL.Path); m != nil {
			pathVersion, version := m[1], ""
			if len(m) > 2 {
				version = m[2]
			}

			pathVersion, err := module.EscapePath(pathVersion)
			if err != nil {
				g.fail(w, "failed to escape path", err)
				return
			}

			if pathVersion == internal.ModPathBase {
				http.NotFound(w, r)
				return
			}

			npmPackage, major, _ := module.SplitPathVersion(pathVersion)
			npmPackage = strings.TrimPrefix(npmPackage, npmjsPrefix)

			mctx := moduleContext{
				NpmPackage:       internal.UnEscapePackage(npmPackage),
				PathMajorVersion: major,
				Version:          version,
			}

			route.handler(w, r, mctx)
			return
		}
	}

	http.NotFound(w, r)
}

// Returns a zip file containing the contents of a specific version of a module.
func (g *npmGoModProxy) Zip(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("npmgomodproxy.zip", mctx)

	npmv, err := internal.FetchPackageVersion(mctx.NpmPackage, mctx.Version)
	if err != nil {
		g.fail(w, "failed to fetch package version", err)
		return
	}

	f, err := internal.CreateZipFromVersion(npmv)
	if err != nil {
		g.fail(w, "failed to create module zip", err)
		return
	}
	defer func() {
		f.Close()
		os.RemoveAll(filepath.Dir(f.Name()))
	}()

	// TODO1 cache + cache headers
	http.ServeContent(w, r, f.Name(), time.Now(), f)
}

func (g *npmGoModProxy) encodeVersion(w io.Writer, version internal.Version) {
	info := versionInfo{
		Version: version.Version,
		// TODO1 time
	}
	jsonEnc := json.NewEncoder(w)
	jsonEnc.Encode(info)
}

func (g *npmGoModProxy) fail(w http.ResponseWriter, what string, err error) {
	err = fmt.Errorf("%s: %s", what, err)
	fmt.Println("error:", err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, err.Error())
}

type versionInfo struct {
	Version string    // version string
	Time    time.Time // commit time
}
