package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/consolidation"
	"github.com/appsprout-dev/mnemonic/internal/agent/encoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/config"
	"github.com/appsprout-dev/mnemonic/internal/embedding"
	"github.com/appsprout-dev/mnemonic/internal/logger"
	"github.com/appsprout-dev/mnemonic/internal/store/sqlite"
)

// buildRetrievalConfig maps the central config to the retrieval agent's config struct.
func buildRetrievalConfig(cfg *config.Config) retrieval.RetrievalConfig {
	return retrieval.RetrievalConfig{
		MaxHops:             cfg.Retrieval.MaxHops,
		ActivationThreshold: float32(cfg.Retrieval.ActivationThreshold),
		DecayFactor:         float32(cfg.Retrieval.DecayFactor),
		MaxResults:          cfg.Retrieval.MaxResults,
		MaxToolCalls:        cfg.Retrieval.MaxToolCalls,
		SynthesisMaxTokens:  cfg.Retrieval.SynthesisMaxTokens,
		MergeAlpha:          float32(cfg.Retrieval.MergeAlpha),
		DualHitBonus:        float32(cfg.Retrieval.DualHitBonus),

		FTSCandidateLimit:       cfg.Retrieval.FTSCandidateLimit,
		EmbeddingCandidateLimit: cfg.Retrieval.EmbeddingCandidateLimit,
		PatternSearchLimit:      cfg.Retrieval.PatternSearchLimit,
		AbstractionSearchLimit:  cfg.Retrieval.AbstractionSearchLimit,

		FTSRankWeight:     float32(cfg.Retrieval.FTSRankWeight),
		FTSSalienceWeight: float32(cfg.Retrieval.FTSSalienceWeight),
		DefaultSalience:   float32(cfg.Retrieval.DefaultSalience),

		TimeRangeBaseScore:  float32(cfg.Retrieval.TimeRangeBaseScore),
		TimeRangeSalienceWt: float32(cfg.Retrieval.TimeRangeSalienceWt),

		RecencyBoostWeight:  float32(cfg.Retrieval.RecencyBoostWeight),
		RecencyHalfLifeDays: float32(cfg.Retrieval.RecencyHalfLifeDays),

		ActivityBonusMax:   float32(cfg.Retrieval.ActivityBonusMax),
		ActivityBonusScale: float32(cfg.Retrieval.ActivityBonusScale),

		CriticalBoost:  float32(cfg.Retrieval.CriticalBoost),
		ImportantBoost: float32(cfg.Retrieval.ImportantBoost),

		DiversityLambda:    float32(cfg.Retrieval.DiversityLambda),
		DiversityThreshold: float32(cfg.Retrieval.DiversityThreshold),

		FeedbackWeight: float32(cfg.Retrieval.FeedbackWeight),
		SourceWeights:  convertSourceWeights(cfg.Retrieval.SourceWeights),
		TypeWeights:    convertSourceWeights(cfg.Retrieval.TypeWeights),

		ContextBoostWindowMin: cfg.Perception.RecallBoostWindowMin,
		ContextBoostMax:       float32(cfg.Perception.RecallBoostMax),
		ContextBoostSources:   convertContextBoostSources(cfg.Retrieval.ContextBoostSources),
	}
}

// convertContextBoostSources converts []string to map[string]bool.
func convertContextBoostSources(src []string) map[string]bool {
	if src == nil {
		return nil
	}
	out := make(map[string]bool, len(src))
	for _, s := range src {
		out[s] = true
	}
	return out
}

// convertSourceWeights converts map[string]float64 to map[string]float32.
func convertSourceWeights(src map[string]float64) map[string]float32 {
	if src == nil {
		return nil
	}
	out := make(map[string]float32, len(src))
	for k, v := range src {
		out[k] = float32(v)
	}
	return out
}

// initRuntime loads config, opens store, and initializes logging for CLI commands.
func initRuntime(configPath string) (*config.Config, *sqlite.SQLiteStore, *slog.Logger) {
	cfg, err := config.Load(configPath)
	if err != nil {
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	log, err := logger.New(logger.Config{Level: "warn", Format: "text"})
	if err != nil {
		die(exitGeneral, fmt.Sprintf("initializing logger: %v", err), "")
	}

	_ = cfg.EnsureDataDir()

	db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
	if err != nil {
		die(exitDatabase, fmt.Sprintf("opening database: %v", err), "mnemonic diagnose")
	}

	return cfg, db, log
}

// initEmbeddingRuntime is like initRuntime but also creates an embedding.Provider.
// Used by CLI commands that create agents.
func initEmbeddingRuntime(configPath string) (*config.Config, *sqlite.SQLiteStore, embedding.Provider, *slog.Logger) {
	cfg, db, log := initRuntime(configPath)
	embProv := newEmbeddingProvider(cfg)
	return cfg, db, embProv, log
}

// initEmbeddingRuntimeMCP is like initEmbeddingRuntime but forces all logging to
// stderr so that stdout remains clean for MCP JSON-RPC framing.
func initEmbeddingRuntimeMCP(configPath string) (*config.Config, *sqlite.SQLiteStore, embedding.Provider, *slog.Logger) {
	cfg, err := config.Load(configPath)
	if err != nil {
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	log, err := logger.New(logger.Config{Level: "warn", Format: "text", Stderr: true})
	if err != nil {
		die(exitGeneral, fmt.Sprintf("initializing logger: %v", err), "")
	}

	// Redirect the global slog default to stderr so that any slog.Info/Warn/Error
	// calls (e.g. in newEmbeddingProvider) don't corrupt stdout.
	slog.SetDefault(log)

	_ = cfg.EnsureDataDir()

	db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
	if err != nil {
		die(exitDatabase, fmt.Sprintf("opening database: %v", err), "mnemonic diagnose")
	}

	embProv := newEmbeddingProvider(cfg)
	return cfg, db, embProv, log
}

// toConsolidationConfig converts the global config's consolidation settings to the agent's config.
func toConsolidationConfig(cfg *config.Config) consolidation.ConsolidationConfig {
	return consolidation.ConsolidationConfig{
		Interval:                  cfg.Consolidation.Interval,
		DecayRate:                 cfg.Consolidation.DecayRate,
		FadeThreshold:             cfg.Consolidation.FadeThreshold,
		ArchiveThreshold:          cfg.Consolidation.ArchiveThreshold,
		RetentionWindow:           cfg.Consolidation.RetentionWindow,
		MaxMemoriesPerCycle:       cfg.Consolidation.MaxMemoriesPerCycle,
		MaxMergesPerCycle:         cfg.Consolidation.MaxMergesPerCycle,
		MinClusterSize:            cfg.Consolidation.MinClusterSize,
		AssocPruneThreshold:       consolidation.DefaultConfig().AssocPruneThreshold,
		RecencyProtection24h:      cfg.Consolidation.RecencyProtection24h,
		RecencyProtection168h:     cfg.Consolidation.RecencyProtection168h,
		AccessResistanceCap:       cfg.Consolidation.AccessResistanceCap,
		AccessResistanceScale:     cfg.Consolidation.AccessResistanceScale,
		MergeSimilarityThreshold:  cfg.Consolidation.MergeSimilarityThreshold,
		PatternMatchThreshold:     cfg.Consolidation.PatternMatchThreshold,
		PatternStrengthIncrement:  float32(cfg.Consolidation.PatternStrengthIncrement),
		PatternIncrementCap:       float32(cfg.Consolidation.PatternIncrementCap),
		LargeClusterBonus:         float32(cfg.Consolidation.LargeClusterBonus),
		LargeClusterMinSize:       cfg.Consolidation.LargeClusterMinSize,
		PatternStrengthCeiling:    float32(cfg.Consolidation.PatternStrengthCeiling),
		StrongEvidenceCeiling:     float32(cfg.Consolidation.StrongEvidenceCeiling),
		StrongEvidenceMinCount:    cfg.Consolidation.StrongEvidenceMinCount,
		PatternBaselineDecay:      float32(cfg.Consolidation.PatternBaselineDecay),
		StaleDecayHealthy:         float32(cfg.Consolidation.StaleDecayHealthy),
		StaleDecayModerate:        float32(cfg.Consolidation.StaleDecayModerate),
		StaleDecayAggressive:      float32(cfg.Consolidation.StaleDecayAggressive),
		SelfSustainingMinEvidence: cfg.Consolidation.SelfSustainingMinEvidence,
		SelfSustainingMinStrength: float32(cfg.Consolidation.SelfSustainingMinStrength),
		SelfSustainingDecay:       float32(cfg.Consolidation.SelfSustainingDecay),
		NeverRecalledArchiveDays:  cfg.Consolidation.NeverRecalledArchiveDays,
		StartupDelay:              time.Duration(cfg.Consolidation.StartupDelaySec) * time.Second,
	}
}

// buildEncodingConfig translates central config into the encoding agent's config struct.
func buildEncodingConfig(cfg *config.Config) encoding.EncodingConfig {
	pollingInterval := time.Duration(cfg.Encoding.PollingIntervalSec) * time.Second
	if pollingInterval <= 0 {
		pollingInterval = 5 * time.Second
	}
	simThreshold := float32(cfg.Encoding.SimilarityThreshold)
	if simThreshold <= 0 {
		simThreshold = 0.3
	}
	return encoding.EncodingConfig{
		PollingInterval:         pollingInterval,
		SimilarityThreshold:     simThreshold,
		MaxSimilarSearchResults: cfg.Encoding.FindSimilarLimit,
		CompletionMaxTokens:     cfg.Encoding.CompletionMaxTokens,
		CompletionTemperature:   float32(cfg.LLM.Temperature),
		MaxConcurrentEncodings:  cfg.Encoding.MaxConcurrentEncodings,
		EnableLLMClassification: cfg.Encoding.EnableLLMClassification,
		CoachingFile:            cfg.Coaching.CoachingFile,
		ExcludePatterns:         cfg.Perception.Filesystem.ExcludePatterns,
		ConceptVocabulary:       cfg.Encoding.ConceptVocabulary,
		MaxRetries:              cfg.Encoding.MaxRetries,
		MaxLLMContentChars:      cfg.Encoding.MaxLLMContentChars,
		MaxEmbeddingChars:       cfg.Encoding.MaxEmbeddingChars,
		TemporalWindowMin:       cfg.Encoding.TemporalWindowMin,
		BackoffThreshold:        cfg.Encoding.BackoffThreshold,
		BackoffBaseSec:          cfg.Encoding.BackoffBaseSec,
		BackoffMaxSec:           cfg.Encoding.BackoffMaxSec,
		BatchSizeEvent:          cfg.Encoding.BatchSizeEvent,
		BatchSizePoll:           cfg.Encoding.BatchSizePoll,
		DeduplicationThreshold:  float32(cfg.Encoding.DeduplicationThreshold),
		SalienceFloor:           cfg.Encoding.SalienceFloor,
	}
}

// newEmbeddingProvider creates an embedding.Provider based on config.
// Priority: explicit embedding config > fallback to LLM config > default BowProvider.
func newEmbeddingProvider(cfg *config.Config) embedding.Provider {
	provider := cfg.Embedding.Provider

	// Explicit "bow" selection — air-gapped mode
	if provider == "bow" {
		slog.Info("embedding provider: bow (128-dim bag-of-words, air-gapped)")
		return embedding.NewBowProvider()
	}

	// Explicit "hugot" selection — pure Go transformer embeddings (MiniLM-L6-v2, 384-dim)
	if provider == "hugot" {
		hugotCfg := embedding.HugotConfig{
			ModelDir:     cfg.Embedding.Model, // repurpose model field as dir path
			AutoDownload: true,
		}
		hp, err := embedding.NewHugotProvider(hugotCfg, slog.Default())
		if err != nil {
			slog.Error("failed to create hugot provider, falling back to bow", "error", err)
			return embedding.NewBowProvider()
		}
		return hp
	}

	// Explicit "api" selection — use embedding-specific config or fall back to LLM config
	if provider == "api" {
		endpoint := cfg.Embedding.Endpoint
		model := cfg.Embedding.Model
		if endpoint == "" {
			endpoint = cfg.LLM.Endpoint
		}
		if model == "" {
			model = cfg.LLM.EmbeddingModel
		}
		if endpoint == "" || model == "" {
			slog.Warn("embedding provider 'api' selected but no endpoint/model configured, falling back to bow")
			return embedding.NewBowProvider()
		}
		timeout := time.Duration(cfg.LLM.TimeoutSec) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		slog.Info("embedding provider: api", "endpoint", endpoint, "model", model)
		return embedding.NewAPIProvider(endpoint, model, cfg.LLM.APIKey, timeout, cfg.LLM.MaxConcurrent)
	}

	// No explicit provider — auto-detect from LLM config for backward compat
	if cfg.LLM.Endpoint != "" && cfg.LLM.EmbeddingModel != "" {
		timeout := time.Duration(cfg.LLM.TimeoutSec) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		slog.Info("embedding provider: api (auto-detected from llm config)", "endpoint", cfg.LLM.Endpoint, "model", cfg.LLM.EmbeddingModel)
		return embedding.NewAPIProvider(cfg.LLM.Endpoint, cfg.LLM.EmbeddingModel, cfg.LLM.APIKey, timeout, cfg.LLM.MaxConcurrent)
	}

	// Default: bag-of-words (zero config, always works, fully air-gapped)
	slog.Info("embedding provider: bow (default, 128-dim bag-of-words)")
	return embedding.NewBowProvider()
}
