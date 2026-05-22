package protocol

// PromptInstructions contains the LLM constraints for each prompt.
type PromptInstructions struct {
	SchemaAConstraint string
	SchemaBConstraint string
}

// DefaultInstructions contains the standard OSS code-focus constraints.
var DefaultInstructions = InstructionsForFocus(FocusCode)

// InstructionsForFocus returns the prompt contract for the supplied focus.
func InstructionsForFocus(focus Focus) PromptInstructions {
	switch NormalizeFocus(focus) {
	case FocusReview:
		return PromptInstructions{
			SchemaAConstraint: reviewSchemaAConstraint,
			SchemaBConstraint: reviewSchemaBConstraint,
		}
	case FocusDesign:
		return PromptInstructions{
			SchemaAConstraint: designSchemaAConstraint,
			SchemaBConstraint: designSchemaBConstraint,
		}
	default:
		return PromptInstructions{
			SchemaAConstraint: codeSchemaAConstraint,
			SchemaBConstraint: codeSchemaBConstraint,
		}
	}
}

const codeSchemaAConstraint = "[TASK]\n%s\n\n[CONSTRAINT]\n" +
	"Do not solve this yet. You do not know exact method names. Output ONLY a machine-readable list using the exact operators below. Output zero other text, explanations, or markdown formatting.\n" +
	"Target the smallest context set required for the first logical step. Prefer 3-7 entries; exceed 10 only if the immediate step clearly requires broad implementation context.\n" +
	"For planning, explanation, triage, or \"what is this project\" queries, request overview files first: entrypoints, public facade/API files, config/defaults, specs/docs if listed, and core orchestrators. Do not request one file from every package just because the query is broad.\n" +
	"FILE:<path>\n" +
	"PREFIX:<path>#<literal prefix from the start of the target line>\n" +
	"NEAR:<path>#<literal string from a nearby unique line or comment>\n"

const reviewSchemaAConstraint = "[TASK]\n%s\n\n[CONSTRAINT]\n" +
	"Do not propose a fix yet. Output ONLY a machine-readable list of review targets or findings using the exact operators below. Output zero other text, explanations, or markdown formatting.\n" +
	"Target the smallest context set needed to confirm or refute correctness, regressions, test coverage gaps, and behavior changes. Prefer changed files, entrypoints, and directly related tests before broader context. Do not request one file from every package just because the change is large.\n" +
	"For review, explanation, or triage queries, request the most relevant implementation and verification files first: entrypoints, modified code paths, tests, and core orchestrators. If the issue appears localized, keep the context narrow.\n" +
	"FILE:<path>\n" +
	"PREFIX:<path>#<literal prefix from the start of the target line>\n" +
	"NEAR:<path>#<literal string from a nearby unique line or comment>\n"

const designSchemaAConstraint = "[TASK]\n%s\n\n[CONSTRAINT]\n" +
	"Do not implement the design yet. Output ONLY a machine-readable list using the exact operators below. Output zero other text, explanations, or markdown formatting.\n" +
	"Target the smallest context set needed to shape the design. Prefer entrypoints, public facade/API files, core models, config/defaults, and specs/docs when present. Do not request one file from every package just because the query is broad.\n" +
	"For planning, explanation, or architecture queries, request overview files first: entrypoints, public facades, core data models, config/defaults, and any relevant specs or docs. Keep the list focused on the contracts that would be changed.\n" +
	"FILE:<path>\n" +
	"PREFIX:<path>#<literal prefix from the start of the target line>\n" +
	"NEAR:<path>#<literal string from a nearby unique line or comment>\n"

const codeSchemaBConstraint = "\n[TASK]\n%s\n\n[OUTPUT CONSTRAINT]\n" +
	"This is the final-answer step. Source context has already been extracted.\n" +
	"Based ONLY on the provided [CONTEXT] and [PROJECT TOPOLOGY], fulfill the [TASK].\n" +
	"Do NOT respond with FILE:, PREFIX:, or NEAR: lines; those selector operators are only for Prompt 1 responses.\n" +
	"\n" +
	"Output format rules:\n" +
	"1. For updated or new files:\n" +
	"--- File: <path/from/project_root> ---\n" +
	"<full updated file contents>\n" +
	"--- End File ---\n\n" +
	"2. For explicit file deletion:\n" +
	"--- Delete File: <path/from/project_root> ---\n\n" +
	"3. For non-code responses: Just write the text normally.\n"

const reviewSchemaBConstraint = "\n[TASK]\n%s\n\n[OUTPUT CONSTRAINT]\n" +
	"This is the final-answer step for a code review.\n" +
	"Based ONLY on the provided [CONTEXT] and [PROJECT TOPOLOGY], report findings, risks, or a clear no-issues result.\n" +
	"Do NOT respond with FILE:, PREFIX:, or NEAR: lines; those selector operators are only for Prompt 1 responses.\n" +
	"\n" +
	"Output format rules:\n" +
	"1. For findings, use concise bullets that include severity, file, and rationale.\n" +
	"2. If no issues are found, state that clearly.\n" +
	"3. Do not invent patches unless the user explicitly asks for a fix.\n"

const designSchemaBConstraint = "\n[TASK]\n%s\n\n[OUTPUT CONSTRAINT]\n" +
	"This is the final-answer step for a design task.\n" +
	"Based ONLY on the provided [CONTEXT] and [PROJECT TOPOLOGY], explain the recommended approach, tradeoffs, or open decisions.\n" +
	"Do NOT respond with FILE:, PREFIX:, or NEAR: lines; those selector operators are only for Prompt 1 responses.\n" +
	"\n" +
	"Output format rules:\n" +
	"1. State the recommended design first.\n" +
	"2. Call out important tradeoffs or follow-up decisions only when they materially affect the design.\n" +
	"3. For non-code responses: Just write the text normally.\n"
