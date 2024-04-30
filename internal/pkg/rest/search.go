package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/arkadyb/climate_mate/internal/pkg/app"
	log "github.com/sirupsen/logrus"
)

func DocumentSearchEndpoint(application *app.App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// validate the request
		query := r.URL.Query().Get("q")
		if len(query) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"missing query(q) parameter"}`)
			return
		}

		numDocuments := numberOfPagesToLook
		numOfDocsToRetrieveParam := r.URL.Query().Get("n")
		if len(numOfDocsToRetrieveParam) > 0 {
			if iVal, err := strconv.Atoi(numOfDocsToRetrieveParam); err == nil {
				numDocuments = iVal
			}
		}

		searchStrategy := app.SearchStrategyTopFirst
		searchStrategyParam := r.URL.Query().Get("searchby")
		if len(searchStrategyParam) > 0 {
			switch searchStrategyParam {
			case "wide":
				searchStrategy = app.SearchStrategyWide
			}
		}

		// search pages
		pages, err := application.Search(r.Context(), query, numDocuments, searchStrategy)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, fmt.Sprintf(`{"message":"failed to find documents for query: '%s'"}`, query))
			return
		}
		w.WriteHeader(http.StatusOK)
		pagesJson, err := json.Marshal(pages.Entries)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"failed to process request"}`)
			log.Error(err)
			return
		}
		io.WriteString(w, fmt.Sprintf(`{"pages":[
			%s
		]}`, string(pagesJson)))
	})
}
