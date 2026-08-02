package http

import (
	stdhttp "net/http"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(NewSelectionUploader, NewClassroomJSONSvc, NewHandler)

type Handler struct{ mux *stdhttp.ServeMux }

func NewHandler(selection *SelectionUploader, classrooms *ClassroomJSONSvc) *Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/class_selection/upload", selection.UploadSelection)
	mux.HandleFunc("/classroom/list", classrooms.GetClassrooms)
	return &Handler{mux: mux}
}

func (h *Handler) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) { h.mux.ServeHTTP(w, r) }
