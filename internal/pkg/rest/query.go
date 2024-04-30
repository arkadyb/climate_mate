package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/arkadyb/climate_mate/internal/pkg/app"
	"github.com/arkadyb/climate_mate/internal/pkg/app/model"
	log "github.com/sirupsen/logrus"
)

const numberOfPagesToLook int = 10

func QueryEndpoint(application *app.App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") //TODO: remove
		// validate the request
		query := r.URL.Query().Get("q")
		if len(query) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"missing query(q) parameter"}`)
			return
		}

		searchStrategy := app.SearchStrategyTopFirst
		searchStrategyParam := r.URL.Query().Get("searchby")
		if len(searchStrategyParam) > 0 {
			switch searchStrategyParam {
			case "wide":
				searchStrategy = app.SearchStrategyWide
				break
			}
		}

		promptToRephrase := "Your query is too short or unclear. Please rephrase your question and try again."

		// Given the following user query and conversation log, formulate a question that would be the most relevant to provide the user with an answer from a knowledge base.\n\nCONVERSATION LOG: \n{conversation}\n\nQuery: {query}\n\nRefined Query:",
		generatedPrompt, err := application.GenerateFromSinglePrompt(
			ctx,
			fmt.Sprintf(
				`Generate a prompt for the user query that would be the most complete to provide the user with the answer from the knowledge base. User query: %s.
				If the query is too short or unclear return '%s'. Return only the generated prompt.`, query, promptToRephrase),
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"failed to process the query"}`)
			log.Error(err)
			return
		}

		answer := struct {
			Answer  string                     `json:"answer"`
			Prompt  string                     `json:"improved_prompt,omitempty"`
			Sources []model.SearchResultsEntry `json:"sources,omitempty"`
		}{}
		if generatedPrompt != promptToRephrase {
			// search pageContents
			searchResults, err := application.Search(r.Context(), generatedPrompt, numberOfPagesToLook, searchStrategy)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, fmt.Sprintf(`{"message":"failed to find documents for query: '%s'"}`, query))
				return
			}

			pages := []string{}
			for _, entry := range searchResults.Entries {
				pages = append(pages, entry.PageContent)
			}

			dunnoAnswer := "I dont know."
			prompts := append([]string{}, pages...)
			prompts = append(prompts, fmt.Sprintf("Answer the question '%s' using the provided context. The answer should not exceed 500 characters. Do not add any formatting, new lines or the special characters. If its impossible to answer reply '%s'.", generatedPrompt, dunnoAnswer))

			answerResp, err := application.GenerateFromParts(ctx, prompts)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, `{"message":"failed to generate answer"}`)
				log.Error(err)
				return
			}
			if answerResp == dunnoAnswer {
				answerResp, err = application.GenerateFromSinglePrompt(ctx, fmt.Sprintf("Answer the user's question '%s'. Do not add any formatting, new lines or the special characters. If its impossible to answer explain the user why. The answer should not exceed 500 characters.", generatedPrompt))
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					io.WriteString(w, `{"message":"failed to generate answer"}`)
					log.Error(err)
					return
				}
				answerResp = fmt.Sprintf(`I couldn't locate an answer within our local knowledge base. Here's what the global knowledge base contains instead. %s`, answerResp)
			}
			answer.Answer = answerResp
			answer.Prompt = generatedPrompt
			answer.Sources = searchResults.Entries
		} else {
			answer.Answer = promptToRephrase
		}

		answerJson, err := json.Marshal(answer)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"failed to construct the sponse"}`)
			log.Error(err)
			return
		}

		io.WriteString(w, string(answerJson))
		w.WriteHeader(http.StatusOK)
	})
}
