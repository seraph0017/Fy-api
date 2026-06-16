package model

type VideoRequestAnalysis struct {
	RequestedResolution string  `json:"requested_resolution,omitempty"`
	Ratio               string  `json:"ratio,omitempty"`
	ReferenceCount      int     `json:"reference_count,omitempty"`
	HasVideoReference   bool    `json:"has_video_reference,omitempty"`
	Storyboard          bool    `json:"storyboard,omitempty"`
	ShotCount           int     `json:"shot_count,omitempty"`
	MotionClass         string  `json:"motion_class,omitempty"`
	AnalysisConfidence  float64 `json:"analysis_confidence,omitempty"`
	ReasonCode          string  `json:"reason_code,omitempty"`
}

type MediaCostEstimateDetail struct {
	Component      string                 `json:"component,omitempty"`
	Provider       string                 `json:"provider,omitempty"`
	Model          string                 `json:"model,omitempty"`
	FormulaKey     string                 `json:"formula_key,omitempty"`
	FormulaVersion string                 `json:"formula_version,omitempty"`
	FormulaText    string                 `json:"formula_text,omitempty"`
	Variables      map[string]interface{} `json:"variables,omitempty"`
	Coefficients   map[string]interface{} `json:"coefficients,omitempty"`
	UsageSource    string                 `json:"usage_source,omitempty"`
	CostRMB        float64                `json:"cost_rmb,omitempty"`
	CostQuota      int                    `json:"cost_quota,omitempty"`
	Currency       string                 `json:"currency,omitempty"`
	RMBPerUSD      float64                `json:"rmb_per_usd,omitempty"`
	QuotaPerUnit   float64                `json:"quota_per_unit,omitempty"`
	Confidence     string                 `json:"confidence,omitempty"`
}

type SeedanceEnhancePipeline struct {
	Pipeline                       string                   `json:"pipeline,omitempty"`
	Status                         string                   `json:"status,omitempty"`
	RequestedResolution            string                   `json:"requested_resolution,omitempty"`
	GenerationResolution           string                   `json:"generation_resolution,omitempty"`
	EnhanceTargetResolution        string                   `json:"enhance_target_resolution,omitempty"`
	Ratio                          string                   `json:"ratio,omitempty"`
	Strategy                       string                   `json:"strategy,omitempty"`
	StrategyVersion                string                   `json:"strategy_version,omitempty"`
	Fallback                       string                   `json:"fallback,omitempty"`
	Analysis                       VideoRequestAnalysis     `json:"analysis,omitempty"`
	MatchedGenerationPolicy        string                   `json:"matched_generation_policy,omitempty"`
	MatchedEnhancePolicy           string                   `json:"matched_enhance_policy,omitempty"`
	MatchReason                    string                   `json:"match_reason,omitempty"`
	MappedFields                   []string                 `json:"mapped_fields,omitempty"`
	DroppedFields                  []string                 `json:"dropped_fields,omitempty"`
	UserRequestedModel             string                   `json:"user_requested_model,omitempty"`
	GenerationProvider             string                   `json:"generation_provider,omitempty"`
	GenerationModel                string                   `json:"generation_model,omitempty"`
	GenerationChannelID            int                      `json:"generation_channel_id,omitempty"`
	GenerationTaskID               string                   `json:"generation_task_id,omitempty"`
	GenerationVideoURL             string                   `json:"generation_video_url,omitempty"`
	EnhanceProvider                string                   `json:"enhance_provider,omitempty"`
	EnhanceTaskID                  string                   `json:"enhance_task_id,omitempty"`
	EnhancedVideoURL               string                   `json:"enhanced_video_url,omitempty"`
	EnhanceRequestID               string                   `json:"enhance_request_id,omitempty"`
	EnhanceError                   string                   `json:"enhance_error,omitempty"`
	ActualDurationSeconds          float64                  `json:"actual_duration_seconds,omitempty"`
	ActualFPS                      float64                  `json:"actual_fps,omitempty"`
	CostPriceVersion               string                   `json:"cost_price_version,omitempty"`
	RMBPerUSD                      float64                  `json:"rmb_per_usd,omitempty"`
	GenerationUsageSource          string                   `json:"generation_usage_source,omitempty"`
	GenerationBillableTokens       int64                    `json:"generation_billable_tokens,omitempty"`
	GenerationEstimatedTokens      int64                    `json:"generation_estimated_tokens,omitempty"`
	GenerationMinimumTokensApplied bool                     `json:"generation_minimum_tokens_applied,omitempty"`
	GenerationCostRMB              float64                  `json:"generation_cost_rmb,omitempty"`
	GenerationCostQuota            int                      `json:"generation_cost_quota,omitempty"`
	EnhanceBillingVersion          string                   `json:"enhance_billing_version,omitempty"`
	EnhanceToolVersion             string                   `json:"enhance_tool_version,omitempty"`
	EnhanceScene                   string                   `json:"enhance_scene,omitempty"`
	EnhanceOutputResolution        string                   `json:"enhance_output_resolution,omitempty"`
	EnhanceOutputFPS               float64                  `json:"enhance_output_fps,omitempty"`
	EnhanceBasePriceRMBPerMinute   float64                  `json:"enhance_base_price_rmb_per_minute,omitempty"`
	EnhanceBillingCoefficient      float64                  `json:"enhance_billing_coefficient,omitempty"`
	EnhanceProviderTaskType        string                   `json:"enhance_provider_task_type,omitempty"`
	EnhanceTaskClass               string                   `json:"enhance_task_class,omitempty"`
	EnhanceTaskClassSource         string                   `json:"enhance_task_class_source,omitempty"`
	EnhanceCostRMB                 float64                  `json:"enhance_cost_rmb,omitempty"`
	EnhanceCostQuota               int                      `json:"enhance_cost_quota,omitempty"`
	PipelineProviderCost           int                      `json:"pipeline_provider_cost_quota,omitempty"`
	UserBilledQuota                int                      `json:"user_billed_quota,omitempty"`
	GrossProfitQuota               int                      `json:"gross_profit_quota,omitempty"`
	GrossMargin                    float64                  `json:"gross_margin,omitempty"`
	GenerationCostDetail           *MediaCostEstimateDetail `json:"generation_cost_detail,omitempty"`
	EnhanceCostDetail              *MediaCostEstimateDetail `json:"enhance_cost_detail,omitempty"`
}
