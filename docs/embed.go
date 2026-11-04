package docs

import "embed"

//go:embed *.md
var Files embed.FS

func Read(name string) string {
	data, err := Files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(data)
}
