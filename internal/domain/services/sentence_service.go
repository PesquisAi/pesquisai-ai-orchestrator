package services

import (
	"context"
	"fmt"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/config/errortypes"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/config/properties"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/delivery/dtos"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/builder"
	enumactions "github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/enums/actions"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/interfaces"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/models"
	nosqlmodels "github.com/PesquisAi/pesquisai-database-lib/nosql/models"
	enumlanguages "github.com/PesquisAi/pesquisai-database-lib/sql/enums/languages"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"slices"
	"strings"
)

const (
	sentenceQuestionTemplate = `You are a part of a major project. In this project I will perform a google search, and your only` +
		` responsibility is to answer me, given the context of the pearson/company that are asking, the desired research` +
		` and the countries that will be used filter the results, what are the best languages that I should use to filter the Google search results. You should answer with a list of 2 digit ` +
		`language codes. Respond only with a comma separated list of language codes, nothing else. ` +
		`Here I have a list of the codes you can use: %s. context:"%s". research:"%s". countries:"%s".`
)

type sentenceService struct {
	queueGemini            interfaces.Queue
	queueGoogleSearch      interfaces.Queue
	requestRepository      interfaces.RequestRepository
	orchestratorRepository interfaces.OrchestratorRepository
}

func (l sentenceService) validateGeminiResponse(response []string) ([]string, error) {
	var errorMessages, res []string
	for i, split := range response {
		if strings.Contains(split, "-") {
			split = strings.Split(split, "-")[0]
			response[i] = split
		}

		if !slices.Contains(enumlanguages.Languages, split) {
			errorMessages = append(errorMessages, fmt.Sprintf("%s is not a valid language", split))
			continue
		}
		res = append(res, split)
	}

	if len(errorMessages) > 0 {
		return nil, errortypes.NewInvalidAIResponseException(errorMessages...)
	}

	return res, nil
}
func (l sentenceService) validateOrchestratorData(request nosqlmodels.Request) error {
	var messages []string
	if request.Context == nil {
		messages = append(messages, `"context" is required in mongoDB to perform language service`)
	}
	if request.Research == nil {
		messages = append(messages, `"research" is required in mongoDB to perform language service`)
	}
	if request.Locations == nil {
		messages = append(messages, `"locations" is required in mongoDB to perform language service`)
	}
	if len(messages) > 0 {
		return errortypes.NewValidationException(messages...)
	}
	return nil
}

func (l sentenceService) Execute(ctx context.Context, request models.AiOrchestratorRequest) error {
	slog.InfoContext(ctx, "languageService.Execute",
		slog.String("details", "process started"))

	var orchestratorData nosqlmodels.Request
	err := l.orchestratorRepository.GetById(ctx, *request.RequestId, &orchestratorData)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Execute",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	err = l.validateOrchestratorData(orchestratorData)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Execute",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	question := fmt.Sprintf(
		questionTemplate,
		strings.Join(enumlanguages.Languages, ","),
		*orchestratorData.Context,
		*orchestratorData.Research,
		strings.Join(*orchestratorData.Locations, ","),
	)

	b, err := builder.BuildQueueGeminiMessage(
		*request.RequestId,
		question,
		properties.QueueNameAiOrchestratorCallback,
		enumactions.LANGUAGE,
	)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Execute",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	err = l.queueGemini.Publish(ctx, b)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Execute",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	return nil
}

func (l sentenceService) Callback(ctx context.Context, callback models.AiOrchestratorCallbackRequest) error {
	slog.InfoContext(ctx, "languageService.Callback",
		slog.String("details", "process started"))

	languages := strings.Split(strings.ToLower(*callback.Response), ",")
	languages, err := l.validateGeminiResponse(languages)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Callback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	g, groupCtx := errgroup.WithContext(ctx)
	for _, language := range languages {
		g.Go(func() error {
			e := l.requestRepository.RelateLanguage(groupCtx, *callback.RequestId, language)
			if e != nil && strings.Contains(e.Error(), `unique constraint "request_languages_pkey"`) {
				return nil
			}
			return e
		})
	}

	err = g.Wait()
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Callback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	err = l.orchestratorRepository.Update(ctx, *callback.RequestId,
		bson.M{"languages": languages},
	)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Callback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	var (
		b      []byte
		action = enumactions.SENTENCES
	)
	b, err = builder.BuildQueueOrchestratorMessage(dtos.AiOrchestratorRequest{
		RequestId: callback.RequestId,
		Action:    &action,
	})
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Callback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	err = l.queueGoogleSearch.Publish(ctx, b)
	if err != nil {
		slog.ErrorContext(ctx, "languageService.Callback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	slog.InfoContext(ctx, "languageService.Callback",
		slog.String("details", "process finished"))
	return nil
}
func NewSentenceService(queueGemini, queueOrchestrator interfaces.Queue, orchestratorRepository interfaces.OrchestratorRepository, requestRepository interfaces.RequestRepository) interfaces.Service {
	return &languageService{
		queueGemini:            queueGemini,
		requestRepository:      requestRepository,
		orchestratorRepository: orchestratorRepository,
		queueOrchestrator:      queueOrchestrator,
	}
}