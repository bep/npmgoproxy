package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/bep/npmgoproxy/npmgop"
	"golang.org/x/mod/module"
)

func main() {

	handler := &gomodproxy{}

	server := &http.Server{Addr: ":8072", Handler: handler}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			// handle err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	fmt.Println("gomodproxy running ...")

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}

type gomodproxy struct {
}

const npmjsPrefix = npmgop.ModPathBase + "/"

var (
	apiList = regexp.MustCompile(`^/(?P<module>.*)/@v/list$`)
	apiInfo = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).info$`)
	apiMod  = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).mod$`)
	apiZip  = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).zip$`)
)

type moduleContext struct {
	NpmPackage       string
	Version          string
	PathMajorVersion string
}

func (ctx moduleContext) String() string {
	return fmt.Sprintf("%s|%s|%s", ctx.NpmPackage, ctx.Version, ctx.PathMajorVersion)
}

func (g *gomodproxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		{"list", apiList, g.list},
		{"info", apiInfo, g.info},
		{"gomodproxy", apiMod, g.mod},
		{"zip", apiZip, g.zip},
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

			if pathVersion == npmgop.ModPathBase {
				http.NotFound(w, r)
				return
			}

			npmPackage, major, _ := module.SplitPathVersion(pathVersion)
			npmPackage = strings.TrimPrefix(npmPackage, npmjsPrefix)

			mctx := moduleContext{
				NpmPackage:       npmPackage,
				PathMajorVersion: major,
				Version:          version,
			}

			if r.Method == http.MethodDelete && version != "" {
				g.delete(w, r, mctx)
				return
			}

			route.handler(w, r, mctx)
			return
		}
	}

	http.NotFound(w, r)
}

/*func (g *gomodproxy) module(ctx context.Context, module string, version vcs.Version) ([]byte, time.Time, error) {
	return []byte("TODO(module)"), time.Now(), nil
}*/

// $base/$module/@v/list
// Returns a list of known versions of the given module in plain text, one per line.
// This list should not include pseudo-versions.
func (g *gomodproxy) list(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("gomodproxy.list", mctx)

	npmpkg, err := npmgop.FetchPackage(mctx.NpmPackage)
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

func (g *gomodproxy) info(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("gomodproxy.info", mctx)

	npmv, err := npmgop.FetchPackageVersion(mctx.NpmPackage, mctx.Version)
	if err != nil {
		g.fail(w, "failed to fetch package version", err)
		return
	}

	info := versionInfo{
		Version: npmv.Version,
		// TODO1 time
	}
	jsonEnc := json.NewEncoder(w)
	jsonEnc.Encode(info)

}

func (g *gomodproxy) mod(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("gomodproxy.mod", mctx)

	// TODO1 deps
	gomod := `

module gohugo.io/npmjs/%s

go 1.17
	
	
`
	fmt.Fprint(w, fmt.Sprintf(gomod, path.Join(mctx.NpmPackage, mctx.PathMajorVersion)))

}

func (g *gomodproxy) zip(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("gomodproxy.zip", mctx)

	npmv, err := npmgop.FetchPackageVersion(mctx.NpmPackage, mctx.Version)
	if err != nil {
		g.fail(w, "failed to fetch package version", err)
		return
	}

	f, err := npmgop.CreateZipFromVersion(npmv)
	if err != nil {
		g.fail(w, "failed to create module zip", err)
		return
	}
	defer f.Close()

	// TODO1 cache + cache headers
	http.ServeContent(w, r, f.Name(), time.Now(), f)

}

func (g *gomodproxy) delete(w http.ResponseWriter, r *http.Request, mctx moduleContext) {
	fmt.Println("gomodproxy.delete")

}

func (g *gomodproxy) fail(w http.ResponseWriter, what string, err error) {
	err = fmt.Errorf("%s: %s", what, err)
	fmt.Println("error:", err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, err.Error())
}

type versionInfo struct {
	Version string    // version string
	Time    time.Time // commit time
}
