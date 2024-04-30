package rest

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"code.sajari.com/docconv"
	"github.com/arkadyb/climate_mate/internal/pkg/app"
	log "github.com/sirupsen/logrus"
)

func DocumentUploadEndpoint(a *app.App) http.Handler {
	// NOTE: the endpoint is protected in the deployed app
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		file, handler, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"failed to read multipart form data"}`)
			return
		}
		defer file.Close()

		summary := r.FormValue("summary")
		if len(summary) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"missing summary"}`)
			return
		}

		docConvResponse, err := docconv.Convert(file, handler.Header["Content-Type"][0], true)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"could parse file"}`)
			log.Error(err)
			return
		}
		if docConvResponse == nil || len(docConvResponse.Error) > 0 {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, fmt.Sprintf(`{"message":"failed to process file. Error: %s"}`, docConvResponse.Error))
			return
		}

		// index the summary into the base collection; index regardless of the size
		err = a.IndexSummaryForFile(r.Context(), handler.Filename, strings.NewReader(summary))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"failed to index the document"}`)
			return
		}

		// index the docs in the collection named same as file; index when the page size is at least 250 chars in length
		err = a.IndexFile(r.Context(), handler.Filename, strings.NewReader(docConvResponse.Body))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"failed to index the document"}`)
			return
		}
	})
}
