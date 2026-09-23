package main

import "testing"

func TestJobLogfCanBeSilencedExplicitly(t *testing.T) {
	original := jobLogf
	t.Cleanup(func() { jobLogf = original })

	called := false
	jobLogf = func(string, ...any) { called = true }
	jobLogf("submit job=%s", "job-1")
	if !called {
		t.Fatal("jobLogf override was not used")
	}
}

func TestColorKeyValueMessageTreatsCommandAsOpaque(t *testing.T) {
	message := "submitted project=demo command=[sh -c echo failing local job\nmarker=\"${ROTARI_BASEDIR}/example-failing-job-marker\"\nif [ ! -f \"${marker}\" ]; then touch \"${marker}\"; exit 1; fi]"
	colored := colorKeyValueMessage(message, func(text string) string { return "G{" + text + "}" })

	want := "G{submitted }G{project}=demoG{ }G{command}=[sh -c echo failing local job\nmarker=\"${ROTARI_BASEDIR}/example-failing-job-marker\"\nif [ ! -f \"${marker}\" ]; then touch \"${marker}\"; exit 1; fi]"
	if colored != want {
		t.Fatalf("colorKeyValueMessage() = %q, want %q", colored, want)
	}
}
