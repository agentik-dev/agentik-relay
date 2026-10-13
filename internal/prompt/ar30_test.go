package prompt_test

import (
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/prompt"
)

// TestAR30_1_BlockParse (AR30 act 1): one marker pair among prose gives
// its lines, the markers included; no markers, nil; two BEGINs, an END
// first or a lone END, an error; indented markers match.
func TestAR30_1_BlockParse(t *testing.T) {
	block := prompt.BlockBegin + "\nsource line\n\n1. **Entry.** Verbatim.\n" + prompt.BlockEnd + "\n"
	md := "# Title\n\nprose before\n" + block + "prose after\n"
	if got, err := prompt.Block([]byte(md)); err != nil || string(got) != block {
		t.Fatalf("one pair: %q, %v; want %q", got, err, block)
	}
	if got, err := prompt.Block([]byte("# Title\n\nno markers here\n")); got != nil || err != nil {
		t.Fatalf("no markers: %q, %v; want nil", got, err)
	}
	for name, bad := range map[string]string{
		"two BEGINs": prompt.BlockBegin + "\n" + block,
		"END first":  prompt.BlockEnd + "\nx\n" + prompt.BlockBegin + "\n",
		"lone END":   "prose\n" + prompt.BlockEnd + "\n",
		"lone BEGIN": prompt.BlockBegin + "\nprose\n",
	} {
		if got, err := prompt.Block([]byte(bad)); err == nil || got != nil {
			t.Fatalf("%s: %q, %v; want an error", name, got, err)
		}
	}
	indented := "  " + prompt.BlockBegin + "\t\nx\n\t" + prompt.BlockEnd
	if got, err := prompt.Block([]byte("p\n" + indented)); err != nil || string(got) != indented+"\n" {
		t.Fatalf("indented: %q, %v", got, err)
	}
}
