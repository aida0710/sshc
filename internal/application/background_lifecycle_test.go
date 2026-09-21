package application

import (
	"strings"
	"testing"
)

func TestLargeBackgroundCanBeRenamedAndDeleted(t *testing.T) {
	for _, operation := range []string{"rename", "delete"} {
		t.Run(operation, func(t *testing.T) {
			service, _ := newTerminalService(t)
			background, err := service.AddBackground("large", png(strings.Repeat("x", 2<<20)))
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if operation == "rename" {
				_, err = service.RenameBackground(background.Name, "renamed")
			} else {
				err = service.RemoveBackground(background.Name)
			}
			if err != nil {
				t.Fatalf("%s of accepted 2 MiB image: %v", operation, err)
			}
		})
	}
}
