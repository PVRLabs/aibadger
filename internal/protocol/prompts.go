package protocol

// PromptInstructions contains the LLM constraints for each prompt.
type PromptInstructions struct {
	SchemaAConstraint string
	SchemaBConstraint string
}

// DefaultInstructions contains the standard OSS constraints.
var DefaultInstructions = PromptInstructions{
	SchemaAConstraint: "[TASK]\n%s\n\n[CONSTRAINT]\n" +
		"Do not solve this yet. You do not know exact method names. Output ONLY a machine-readable list using the exact operators below. Output zero other text, explanations, or markdown formatting.\n" +
		"Target the smallest context set required for the first logical step. Prefer 3-7 entries; exceed 10 only if the immediate step clearly requires broad implementation context.\n" +
		"For planning, explanation, triage, or \"what is this project\" queries, request overview files first: entrypoints, public facade/API files, config/defaults, specs/docs if listed, and core orchestrators. Do not request one file from every package just because the query is broad.\n" +
		"FILE:<path>\n" +
		"PREFIX:<path>#<literal prefix from the start of the target line>\n" +
		"NEAR:<path>#<literal string from a nearby unique line or comment>\n",

	SchemaBConstraint: "\n[TASK]\n%s\n\n[OUTPUT CONSTRAINT]\n" +
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
		"3. For non-code responses: Just write the text normally.\n",
}
