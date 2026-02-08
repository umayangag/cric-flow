package indexer

type ProjectContext struct {
	Root      string     `json:"root"`
	Structure []FileNode `json:"structure"`
	Stats     Stats      `json:"stats"`
}

type Stats struct {
	Files       int `json:"files"`
	Directories int `json:"directories"`
	GoFiles     int `json:"go_files"`
	PyFiles     int `json:"py_files"`
}

type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"` // Relative path
	Type     string     `json:"type"` // "dir" or "file"
	Children []FileNode `json:"children,omitempty"`
	Symbols  []Symbol   `json:"symbols,omitempty"`
	Content  string     `json:"content,omitempty"` // For small config/readme files
}

type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "function", "class", "variable", "struct"
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"`
	Line      int    `json:"line"`
}
