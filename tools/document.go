package tools

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"

	"agent-gui/domain"
)

// maxDocBytes caps extracted text so a huge document cannot blow up the context.
const maxDocBytes = 1 * 1024 * 1024 // 1 MB of extracted text

// readDocumentTool extracts plain text from PDF and DOCX files.
type readDocumentTool struct{ workDir string }

// NewReadDocument builds the read_document tool, rooted at workDir for safety.
func NewReadDocument(workDir string) domain.Tool { return &readDocumentTool{workDir} }

func (t *readDocumentTool) Name() string { return "read_document" }

func (t *readDocumentTool) Description() string {
	return "Extract plain text from a PDF or DOCX file (returns text with page/section markers)"
}

func (t *readDocumentTool) Parameters() string {
	return `{"path": "path to a .pdf or .docx file (relative to the work directory)"}`
}

func (t *readDocumentTool) Execute(args map[string]string) string {
	abs, err := safePath(args["path"], t.workDir)
	if err != nil {
		return "ERROR: " + err.Error()
	}

	lower := strings.ToLower(abs)
	var text string
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		text, err = extractPDF(abs)
	case strings.HasSuffix(lower, ".docx"):
		text, err = extractDOCX(abs)
	default:
		return "ERROR: unsupported file type — only .pdf and .docx are supported"
	}
	if err != nil {
		return fmt.Sprintf("ERROR: could not read document: %v", err)
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return "Document contained no extractable text (it may be a scanned image-only PDF)."
	}
	if len(text) > maxDocBytes {
		text = text[:maxDocBytes] + "\n\n[... truncated: document exceeds 1 MB of text ...]"
	}
	return text
}

// extractPDF returns the document text with "--- Page N ---" markers per page.
func extractPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var sb strings.Builder
	total := r.NumPage()
	for i := 1; i <= total; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			// Skip unreadable pages rather than failing the whole document.
			continue
		}
		fmt.Fprintf(&sb, "--- Page %d/%d ---\n%s\n\n", i, total, strings.TrimSpace(content))
		if sb.Len() > maxDocBytes {
			break
		}
	}
	return sb.String(), nil
}

// extractDOCX unzips the .docx and pulls text from word/document.xml,
// inserting a blank line between paragraphs.
func extractDOCX(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = zr.Close() }()

	var docXML io.ReadCloser
	for _, file := range zr.File {
		if file.Name == "word/document.xml" {
			docXML, err = file.Open()
			if err != nil {
				return "", err
			}
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("not a valid .docx (missing word/document.xml)")
	}
	defer func() { _ = docXML.Close() }()

	var sb strings.Builder
	dec := xml.NewDecoder(docXML)
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "t": // w:t — a text run
				inText = true
			case "tab":
				sb.WriteByte('\t')
			case "br": // line break
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "t":
				inText = false
			case "p": // end of paragraph
				sb.WriteString("\n\n")
			}
		case xml.CharData:
			if inText {
				sb.Write(el)
			}
		}
		if sb.Len() > maxDocBytes {
			break
		}
	}
	return sb.String(), nil
}
