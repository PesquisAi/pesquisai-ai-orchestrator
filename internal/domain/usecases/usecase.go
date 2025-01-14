package usecases

import (
	"context"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/factory"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/interfaces"
	"github.com/PesquisAi/pesquisai-ai-orchestrator/internal/domain/models"
	"log/slog"
)

type UseCase struct {
	requestRepository      interfaces.RequestRepository
	serviceFactory         *factory.ServiceFactory
	orchestratorRepository interfaces.OrchestratorRepository
}

func (u UseCase) OrchestrateCallback(ctx context.Context, request models.AiOrchestratorCallbackRequest) error {
	slog.InfoContext(ctx, "useCase.OrchestrateCallback",
		slog.String("details", "process started"),
		slog.String("action", *request.Action))

	service, err := u.serviceFactory.Factory(*request.Action)
	if err != nil {
		slog.ErrorContext(ctx, "useCase.OrchestrateCallback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	err = service.Callback(ctx, request)
	if err != nil {
		slog.ErrorContext(ctx, "useCase.OrchestrateCallback",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	slog.DebugContext(ctx, "useCase.OrchestrateCallback",
		slog.String("details", "process finished"))
	return nil
}

func (u UseCase) Orchestrate(ctx context.Context, request models.AiOrchestratorRequest) error {
	slog.InfoContext(ctx, "useCase.Orchestrate",
		slog.String("details", "process started"),
		slog.String("action", *request.Action),
	)

	service, err := u.serviceFactory.Factory(*request.Action)
	if err != nil {
		slog.ErrorContext(ctx, "useCase.Orchestrate",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	err = service.Execute(ctx, request)
	if err != nil {
		slog.ErrorContext(ctx, "useCase.Orchestrate",
			slog.String("details", "process error"),
			slog.String("error", err.Error()))
		return err
	}

	slog.DebugContext(ctx, "useCase.Orchestrate",
		slog.String("details", "process finished"))
	return nil
}

func NewUseCase(requestRepository interfaces.RequestRepository, serviceFactory *factory.ServiceFactory) interfaces.UseCase {
	return &UseCase{
		requestRepository: requestRepository,
		serviceFactory:    serviceFactory,
	}
}
