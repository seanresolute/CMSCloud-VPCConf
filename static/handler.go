package static

import (
	"net/http"
	"strings"
)

type handler struct {
	baseServer http.Handler
}

// A FileServer behaves like http.FileServer, except that
// if the path begins with /static/ver:*/ then it will
// remove the /ver:*/ before serving and set
// Cache-Control: max-age=31556926
func FileServer(fs http.FileSystem) http.Handler {
	return &handler{
		baseServer: http.FileServer(fs),
	}
}

const staticPrefix = "/static/"
const versionPrefix = "ver:"

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, staticPrefix+versionPrefix) {
		slashPos := strings.Index(path[len(staticPrefix):], "/")
		if slashPos != -1 {
			r.URL.Path = staticPrefix + path[len(staticPrefix)+slashPos+1:]
			w.Header().Set("Cache-Control", "max-age=31556926")
		}
	}
	h.baseServer.ServeHTTP(w, r)
}
