package llamacpp

// SystemPromptMarkdown guides the multimodal Large Language Model (e.g., LLaVA, Qwen-VL)
// on how to precisely translate the pixels of a PDF page image into structured markdown.
// It explicitly discourages hallucination and prompts it to respect the original layout.
const SystemPromptMarkdown = `You are a highly capable Document Extraction Engine. 
Your only task is to analyze the provided image of a document page and extract all text and structural 
elements into GitHub Flavored Markdown format.

Adhere strictly to these rules:
1. Extract ALL text exactly as it appears in the image. Do not summarize, add, or invent any information.
2. Preserve reading order rigorously. If the document has multiple columns, read the left column top-to-bottom first, then the right column.
3. Represent all mathematical formulas and equations using valid LaTeX wrapped in double dollar signs ($$equation$$).
4. Preserve headings, paragraphs, and lists using appropriate Markdown syntax (#, ##, -, 1.).
5. If you identify a table, extract it as a standard Markdown table using pipes (|).
6. Do not include any introductory or concluding conversational text. Output ONLY the raw Markdown content.`

// FormatPrompt is a helper to encapsulate standard instruction formats like ChatML.
// Depending on the VLM chosen (e.g. LLaVA-1.5, Qwen, Moondream), the prompt might need
// specialized formatting tokens (e.g., <|im_start|>system\n...<|im_end|>).
func FormatPrompt(systemPrompt string) string {
	// For general instruct-tuned models, a simple system prefix and USER token work.
	// We structure it generically here. If specific VLMs require ChatML or vicuna formats,
	// standard LLaVA uses: "USER: <image>\n[systemPrompt]\nASSISTANT:"

	// Because go-llama.cpp handles the <image> token injection automatically when SetImage() is used:
	return "USER: " + systemPrompt + "\nASSISTANT:"
}
