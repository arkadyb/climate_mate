package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/arkadyb/climate_mate/internal/pkg/app/model"
	"github.com/arkadyb/climate_mate/internal/pkg/config"
	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
	"github.com/tmc/langchaingo/documentloaders"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/pgvector"
	"golang.org/x/exp/slices"
)

const (
	chunkSize    int = 1000
	chunkOverlap int = 100

	numberOfNamespacesToSearchIn int = 2

	DefaultCollectionName       string = "langchain"
	MetadataCollectionFieldName string = "collection_name"
)

type SearchStrategy int

const (
	// this strategy drives to search a top N of documents across <numberOfNamespacesToSearchIn> collections and selects a top N documents based on the score
	SearchStrategyTopFirst SearchStrategy = iota
	// this strategy drives to seach a top N of documents across <numberOfNamespacesToSearchIn> collections and selects top N/<numberOfNamespacesToSearchIn> from each
	SearchStrategyWide
)

func NewApp(ctx context.Context, cfg config.Config) *App {
	llm, err := googleai.New(ctx, googleai.WithAPIKey(cfg.GoogleAiApiKey))
	if err != nil {
		log.Fatal(err)
	}

	conn, err := pgx.Connect(ctx, fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", cfg.PGUserName, cfg.PGPassword, cfg.PGHost, cfg.PGPort, cfg.PGDBName))
	if err != nil {
		log.Fatal(err)
	}
	return &App{
		llm:            llm,
		embedderClient: llm,
		pgconn:         conn,
	}
}

type App struct {
	llm            llms.Model
	embedderClient embeddings.EmbedderClient
	pgconn         *pgx.Conn
}

func loadAndSplit(ctx context.Context, contentReader *strings.Reader, minChunkToIndexSize int) ([]schema.Document, error) {
	docs := []schema.Document{}
	var err error
	documentText := documentloaders.NewText(contentReader)
	if contentReader.Size() > int64(chunkOverlap) {
		docs, err = documentText.LoadAndSplit(
			ctx,
			textsplitter.NewRecursiveCharacter(
				textsplitter.WithChunkSize(chunkSize),
				textsplitter.WithChunkOverlap(chunkOverlap),
			),
		)
		if err != nil {
			log.Error(err)
			return nil, err
		}
	} else {
		docs, err = documentText.Load(ctx)
		if err != nil {
			log.Error(err)
			return nil, err
		}
	}

	cleanedDocs := []schema.Document{}
	// preformat the docs - remove new lines and special chars
	// cleanout the docs < than minChunkToIndexSize; skip if 0
	for i := 0; i < len(docs); i++ {
		if len(docs[i].PageContent) > minChunkToIndexSize || minChunkToIndexSize == 0 {
			docs[i].PageContent = removeLBR(docs[i].PageContent)
			cleanedDocs = append(cleanedDocs, docs[i])
		}
	}
	return cleanedDocs, nil
}

func (app *App) IndexSummaryForFile(ctx context.Context, fileName string, contentReader *strings.Reader) error {
	docs, err := loadAndSplit(ctx, contentReader, 0)
	if err != nil {
		log.Error(err)
		return err
	}

	// set metadata
	metadata := map[string]any{
		MetadataCollectionFieldName: fileName,
	}
	for i := 0; i < len(docs); i++ {
		docs[i].Metadata = metadata
	}

	// when reindexing - check to remove the existing sumary embeddings in the default collection
	_, err = app.pgconn.Exec(ctx, `DELETE
	FROM langchain_pg_embedding AS emb USING langchain_pg_collection AS coll 
	WHERE coll.name=$1 AND emb.cmetadata ->> 'collection_name' = $2`, DefaultCollectionName, fileName)
	if err != nil {
		return err
	}

	store, err := app.createVectorStore(ctx)
	if err != nil {
		return err
	}

	return app.indexText(ctx, store, docs)
}

func (app *App) IndexFile(ctx context.Context, fileName string, contentReader *strings.Reader) error {
	docs, err := loadAndSplit(ctx, contentReader, 250)
	if err != nil {
		log.Error(err)
		return err
	}

	// set metadata
	metadata := map[string]any{
		MetadataCollectionFieldName: fileName,
	}
	for i := 0; i < len(docs); i++ {
		docs[i].Metadata = metadata
	}

	store, err := app.createVectorStoreByName(ctx, fileName)
	if err != nil {
		return err
	}

	err = app.indexText(ctx, store, docs)
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func (app *App) indexText(ctx context.Context, store *pgvector.Store, docs []schema.Document) error {
	dedupMap := make(map[string]struct{})
	dedup := func(ctx context.Context, doc schema.Document) bool {
		if _, ok := dedupMap[doc.PageContent]; ok {
			return true // skip duplicated for the doc
		}
		dedupMap[doc.PageContent] = struct{}{}
		return false
	}
	_, err := store.AddDocuments(ctx, docs, vectorstores.WithDeduplicater(dedup))
	if err != nil {
		return err
	}
	return nil
}

func (app *App) Search(ctx context.Context, query string, numDocuments int, searchStrategy SearchStrategy) (model.SearchResults, error) {
	store, err := app.createVectorStore(ctx)
	if err != nil {
		log.Error(err)
		return model.SearchResults{}, err
	}

	// run initial search in the default namespace - take the best matching the query
	defaultNamespaceDocs, err := store.SimilaritySearch(ctx, query, numberOfNamespacesToSearchIn)
	if err != nil {
		log.Error(err)
		return model.SearchResults{}, err
	}

	uniqueNamespacesMap := map[string]struct{}{}
	for _, defaultNamedefaultNamespaceDoc := range defaultNamespaceDocs {
		namespace := defaultNamedefaultNamespaceDoc.Metadata[MetadataCollectionFieldName].(string)
		if _, ok := uniqueNamespacesMap[namespace]; !ok {
			uniqueNamespacesMap[namespace] = struct{}{}
		}
	}

	docs := []schema.Document{}

	switch searchStrategy {
	case SearchStrategyTopFirst:
		// do the search across in the all the target namespaces and merge the results by score
		for namespace := range uniqueNamespacesMap {
			namespaceDocs, err := store.SimilaritySearch(ctx, query, numDocuments, vectorstores.WithNameSpace(namespace))
			if err != nil {
				log.Error(err)
				return model.SearchResults{}, err
			}
			docs = append(docs, namespaceDocs...)
		}
		// TODO: switch over to use merge strategy instead
		slices.SortFunc(docs, func(a, b schema.Document) int {
			if a.Score < b.Score {
				return -1
			} else if a.Score > b.Score {
				return 1
			}
			return 0
		})
	case SearchStrategyWide:
		// do the search across in the all the target namespaces and merge the results by score
		for namespace := range uniqueNamespacesMap {
			namespaceDocs, err := store.SimilaritySearch(ctx, query, numDocuments, vectorstores.WithNameSpace(namespace))
			if err != nil {
				log.Error(err)
				return model.SearchResults{}, err
			}
			if len(namespaceDocs) > 0 {
				docs = append(docs, namespaceDocs[:len(namespaceDocs)/len(defaultNamespaceDocs)]...)
			}
		}
		slices.SortFunc(docs, func(a, b schema.Document) int {
			if a.Score < b.Score {
				return -1
			} else if a.Score > b.Score {
				return 1
			}
			return 0
		})
	}

	pageResults := []model.SearchResultsEntry{}
	if len(docs) < numDocuments {
		numDocuments = len(docs)
	}
	for _, doc := range docs[:numDocuments] {
		pageResults = append(pageResults, model.SearchResultsEntry{
			Filename:    doc.Metadata[MetadataCollectionFieldName].(string),
			PageContent: doc.PageContent,
			Score:       doc.Score,
		})
	}

	return model.SearchResults{
		Entries: pageResults,
	}, nil
}

func (app *App) GenerateFromSinglePrompt(ctx context.Context, prompt string) (string, error) {
	prompt, err := llms.GenerateFromSinglePrompt(ctx, app.llm, prompt)
	if err != nil {
		log.Error(err)
		return "", err
	}

	return prompt, nil
}

func (app *App) GenerateFromParts(ctx context.Context, prompts []string) (string, error) {
	resp, err := app.llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompts...),
	})
	if err != nil {
		log.Error(err)
		return "", err
	}

	if resp != nil && len(resp.Choices) > 0 {
		return resp.Choices[0].Content, nil
	}

	return "", nil
}

func (app *App) createVectorStoreByName(ctx context.Context, collectionName string) (*pgvector.Store, error) {
	if len(collectionName) == 0 {
		return nil, errors.New("collection name is required")
	}
	emb, err := embeddings.NewEmbedder(app.embedderClient, embeddings.WithStripNewLines(true))
	if err != nil {
		return nil, err
	}
	pgVectorStore, err := pgvector.New(
		ctx,
		pgvector.WithCollectionName(collectionName),
		pgvector.WithPreDeleteCollection(true),
		pgvector.WithConn(app.pgconn),
		pgvector.WithEmbedder(emb),
	)
	if err != nil {
		return nil, err
	}

	return &pgVectorStore, nil
}

func (app *App) createVectorStore(ctx context.Context) (*pgvector.Store, error) {
	emb, err := embeddings.NewEmbedder(app.embedderClient, embeddings.WithStripNewLines(true))
	if err != nil {
		return nil, err
	}
	pgVectorStore, err := pgvector.New(
		ctx,
		pgvector.WithCollectionName(DefaultCollectionName),
		pgvector.WithConn(app.pgconn),
		pgvector.WithEmbedder(emb),
	)

	if err != nil {
		return nil, err
	}

	return &pgVectorStore, nil
}

func removeLBR(text string) string {
	re := regexp.MustCompile(`\x{000D}\x{000A}|[\x{000A}\x{000B}\x{000C}\x{000D}\x{0085}\x{2028}\x{2029}]`)
	return re.ReplaceAllString(text, " ")
}
