package tools

import "encoding/json"

// JSON schemas of the built-in tools, for native tool calling and argument
// validation.

func object(props string, required ...string) json.RawMessage {
	req, _ := json.Marshal(append([]string{}, required...)) // strings cannot fail
	return json.RawMessage(`{"type":"object","properties":{` + props + `},"required":` + string(req) + `}`)
}

func str(name, description string) string {
	return `"` + name + `":{"type":"string","description":"` + description + `"}`
}

func (t *readFileTool) Schema() json.RawMessage {
	return object(str("path", "file path, absolute or relative to the work directory"), "path")
}

func (t *writeFileTool) Schema() json.RawMessage {
	return object(str("path", "file path")+","+str("content", "complete file content"), "path", "content")
}

func (t *listDirTool) Schema() json.RawMessage {
	return object(str("path", "directory, defaults to the work directory"))
}

func (t *searchFilesTool) Schema() json.RawMessage {
	return object(str("pattern", "text to find")+","+str("dir", "directory, defaults to the work directory")+","+
		str("ext", "file extension filter such as .go"), "pattern")
}

func (t *deleteFileTool) Schema() json.RawMessage {
	return object(str("path", "file to delete"), "path")
}

func (t *shellTool) Schema() json.RawMessage {
	return object(str("command", "bash command")+","+str("dir", "directory inside the work directory"), "command")
}

func (t *timeTool) Schema() json.RawMessage { return object("") }

func (t *readDocumentTool) Schema() json.RawMessage {
	return object(str("path", "path to a .pdf or .docx file"), "path")
}

func (t *webSearchTool) Schema() json.RawMessage {
	return object(str("query", "search query")+`,"count":{"type":"integer","description":"number of results, 1-10"}`, "query")
}
