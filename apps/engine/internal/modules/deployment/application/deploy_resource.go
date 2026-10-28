package application

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// DeployedDefinition adalah definition hasil deploy; Duplicate=true bila versi lama dipakai ulang.
type DeployedDefinition struct {
	domain.ProcessDefinition
	Duplicate bool
}

// DeployResult adalah hasil use case DeployResource.
type DeployResult struct {
	DeploymentKey string
	Definitions   []DeployedDefinition
	Warnings      []domain.Violation
}

// DeployResource memvalidasi resource, memberi versi, menyimpan, dan mempublikasikan event.
type DeployResource struct {
	repo  domain.Repository
	bus   eventbus.Bus
	cache *ProcessCache
	clock func() time.Time
	log   *slog.Logger
}

// NewDeployResource merakit use case.
func NewDeployResource(repo domain.Repository, bus eventbus.Bus, cache *ProcessCache, clock func() time.Time, log *slog.Logger) *DeployResource {
	if clock == nil {
		clock = time.Now
	}
	return &DeployResource{repo: repo, bus: bus, cache: cache, clock: clock, log: log}
}

type parsedResource struct {
	resource   domain.Resource
	defs       *bpmn.Definitions
	candidates []domain.ProcessCandidate
}

// parseAndValidate memeriksa semua resource dan mengumpulkan seluruh violation sekaligus.
func parseAndValidate(inputs []domain.ResourceInput, profile bpmn.Profile) ([]parsedResource, []domain.Violation) {
	resources, violations := domain.NewResources(inputs)
	var parsed []parsedResource
	var candidates []domain.ProcessCandidate
	for _, r := range resources {
		defs, issues, err := bpmn.ParseAndValidate(bytes.NewReader(r.Content), profile)
		violations = append(violations, toViolations(r.Name, issues)...)
		if err != nil {
			continue
		}
		if profile.Name() == bpmn.ProfileExecutable.Name() {
			violations = append(violations, checkExpressions(r.Name, defs)...)
		}
		pr := parsedResource{resource: r, defs: defs}
		for _, p := range defs.ExecutableProcesses() {
			c := domain.ProcessCandidate{
				ResourceName: r.Name, BpmnProcessID: p.ID(), Name: p.Name(), VersionTag: p.VersionTag(), Checksum: r.Checksum,
			}
			pr.candidates = append(pr.candidates, c)
			candidates = append(candidates, c)
		}
		parsed = append(parsed, pr)
	}
	violations = append(violations, domain.CheckUniqueProcessIDs(candidates)...)
	return parsed, violations
}

// Execute menjalankan alur deploy. Mengembalikan *domain.RejectedError bila ada violation error.
func (uc *DeployResource) Execute(ctx context.Context, tenantID string, inputs []domain.ResourceInput) (DeployResult, error) {
	if tenantID == "" {
		tenantID = domain.DefaultTenant
	}
	parsed, violations := parseAndValidate(inputs, bpmn.ProfileExecutable)
	if hasErrors(violations) {
		return DeployResult{}, &domain.RejectedError{Violations: violations}
	}

	var (
		result    DeployResult
		newModels = map[string]*bpmn.Process{}
		events    []api.ProcessDeployed
	)
	err := uc.repo.WithTx(ctx, func(ctx context.Context, tx domain.TxRepository) error {
		// Kunci proses dalam urutan id yang terurut agar dua deploy paralel tidak deadlock.
		type candidateRef struct {
			parsed    *parsedResource
			candidate domain.ProcessCandidate
		}
		var refs []candidateRef
		for i := range parsed {
			for _, c := range parsed[i].candidates {
				refs = append(refs, candidateRef{parsed: &parsed[i], candidate: c})
			}
		}
		sort.Slice(refs, func(a, b int) bool { return refs[a].candidate.BpmnProcessID < refs[b].candidate.BpmnProcessID })

		keys, err := tx.NextKeys(ctx, 1+len(refs))
		if err != nil {
			return err
		}
		now := uc.clock()
		dep := domain.Deployment{Key: keys[0], TenantID: tenantID, CreatedAt: now}
		for _, p := range parsed {
			dep.Resources = append(dep.Resources, p.resource)
		}
		nextKey := 1
		for _, ref := range refs {
			c := ref.candidate
			if err := tx.LockProcessID(ctx, tenantID, c.BpmnProcessID); err != nil {
				return err
			}
			latest, found, err := tx.LatestByProcessID(ctx, tenantID, c.BpmnProcessID)
			if err != nil {
				return err
			}
			if found && latest.IsDuplicateOf(c.Checksum) {
				result.Definitions = append(result.Definitions, DeployedDefinition{ProcessDefinition: latest, Duplicate: true})
				continue
			}
			var latestPtr *domain.ProcessDefinition
			if found {
				latestPtr = &latest
			}
			pd := domain.ProcessDefinition{
				Key: keys[nextKey], TenantID: tenantID, BpmnProcessID: c.BpmnProcessID,
				Version: domain.NextVersion(latestPtr), VersionTag: c.VersionTag, Name: c.Name,
				ResourceName: c.ResourceName, Checksum: c.Checksum, DeploymentKey: dep.Key, CreatedAt: now,
			}
			nextKey++
			dep.Definitions = append(dep.Definitions, pd)
			result.Definitions = append(result.Definitions, DeployedDefinition{ProcessDefinition: pd})
			if proc, ok := ref.parsed.defs.Process(c.BpmnProcessID); ok {
				newModels[pd.Key] = proc
			}
			events = append(events, api.ProcessDeployed{
				ProcessDefinitionKey: pd.Key, BpmnProcessID: pd.BpmnProcessID, Version: pd.Version,
				VersionTag: pd.VersionTag, DeploymentKey: dep.Key, TenantID: tenantID,
			})
		}
		result.DeploymentKey = dep.Key
		return tx.SaveDeployment(ctx, dep)
	})
	if err != nil {
		return DeployResult{}, fmt.Errorf("deploy: %w", err)
	}

	// Setelah commit: isi cache dan publikasikan event versi baru.
	for key, proc := range newModels {
		uc.cache.Put(key, proc)
	}
	for _, ev := range events {
		if err := uc.bus.Publish(ctx, ev); err != nil {
			// Handler gagal tidak membatalkan deploy; outbox transaksional menyusul di Fase 3.
			uc.log.Error("publish process deployed", slog.String("bpmn_process_id", ev.BpmnProcessID),
				slog.String("key", ev.ProcessDefinitionKey), slog.Any("error", err))
		}
	}
	result.Warnings = warningsOnly(violations)
	return result, nil
}
