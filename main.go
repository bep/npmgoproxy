package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/mod/module"

	"golang.org/x/mod/zip"
)

// TODO1
//  npm pack --dry-run simple-icons

func main() {

	if false {
		createZip()
		return
	}
	handler := &gomodproxy{}

	server := &http.Server{Addr: ":8072", Handler: handler}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			// handle err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}

type gomodproxy struct {
}

const npmjsPrefix = "/gohugo.io"

var (
	apiList = regexp.MustCompile(`^/(?P<module>.*)/@v/list$`)
	apiInfo = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).info$`)
	apiMod  = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).mod$`)
	apiZip  = regexp.MustCompile(`^/(?P<module>.*)/@v/(?P<version>.*).zip$`)
)

var (
	testMod         = "npmjs"
	testModVersion  = "v1.13.0"
	testModVersions = []string{testModVersion}
)

func (g *gomodproxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !strings.HasPrefix(r.URL.Path, npmjsPrefix) {
		return
	}

	urlPath := strings.TrimPrefix(r.URL.Path, npmjsPrefix)

	for _, route := range []struct {
		id      string
		regexp  *regexp.Regexp
		handler func(w http.ResponseWriter, r *http.Request, module, version string)
	}{
		{"list", apiList, g.list},
		{"info", apiInfo, g.info},
		{"gomodproxy", apiMod, g.mod},
		{"zip", apiZip, g.zip},
	} {
		if m := route.regexp.FindStringSubmatch(urlPath); m != nil {
			module, version := m[1], ""
			if len(m) > 2 {
				version = m[2]
			}
			module = decodeBangs(module)
			if r.Method == http.MethodDelete && version != "" {
				g.delete(w, r, module, version)
				return
			}

			route.handler(w, r, module, version)
			return
		}
	}

	http.NotFound(w, r)
}

/*func (g *gomodproxy) module(ctx context.Context, module string, version vcs.Version) ([]byte, time.Time, error) {
	return []byte("TODO(module)"), time.Now(), nil
}*/

func (g *gomodproxy) list(w http.ResponseWriter, r *http.Request, module, version string) {
	fmt.Println("gomodproxy.list", "module", module, "version", version)
	versions := strings.Join(testModVersions, "\n")
	fmt.Fprint(w, versions)

}

func (g *gomodproxy) info(w http.ResponseWriter, r *http.Request, module, version string) {
	fmt.Println("gomodproxy.info", "module", module, "version", version)

	info := versionInfo{
		Version: testModVersion,
	}
	jsonEnc := json.NewEncoder(w)
	jsonEnc.Encode(info)

}

func (g *gomodproxy) mod(w http.ResponseWriter, r *http.Request, module, version string) {
	fmt.Println("gomodproxy.mod", "module", module, "version", version)

	gomod := `

module gohugo.io/npmjs/simple-icons

go 1.17
	
	
`
	fmt.Fprint(gomod)

}

func (g *gomodproxy) zip(w http.ResponseWriter, r *http.Request, module, version string) {
	fmt.Println("gomodproxy.zip", "module", module, "version", version)
	zipFilename := "simple-icons-5.13.0.zip"
	f, err := os.Open(zipFilename)
	if err != nil {
		g.fail(w, err)
		return
	}
	defer f.Close()
	http.ServeContent(w, r, zipFilename, time.Now(), f)

}

func (g *gomodproxy) delete(w http.ResponseWriter, r *http.Request, module, version string) {
	fmt.Println("gomodproxy.delete", "module", module, "version", version)

}

func (g *gomodproxy) fail(w http.ResponseWriter, err error) {
	fmt.Println("error:", err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, err.Error())
}

func checkZipFile(name, modulePath, moduleVersion string) error {
	_, err := zip.CheckZip(
		module.Version{
			Path:    modulePath,
			Version: moduleVersion,
		},
		name,
	)

	return err
}

func decodeBangs(s string) string {
	buf := []rune{}
	bang := false
	for _, r := range []rune(s) {
		if bang {
			bang = false
			buf = append(buf, unicode.ToUpper(r))
			continue
		}
		if r == '!' {
			bang = true
			continue
		}
		buf = append(buf, r)
	}
	return string(buf)
}

type versionInfo struct {
	Version string    // version string
	Time    time.Time // commit time
}

func createZip() {
	f, err := os.Create("simple-icons-5.13.0.zip")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	err = zip.CreateFromDir(f, module.Version{Path: "gohugo.io/npmjs/simple-icons", Version: testModVersion}, "package")
	if err != nil {
		log.Fatal(err)
	}
}
