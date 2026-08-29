package shellcmd

import "testing"

func TestRunExposesLevels(t *testing.T) {
	if err := Run(`test "$LEVEL" = 42 && test "$RAW_LEVEL" = 107`, 107); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsEmptyCommand(t *testing.T) {
	if err := Run("  ", 0); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}
