package hooks

import (
	"encoding/json"
	"fmt"
	"strings"
)

// maxBlocks is how many times the Stop guard blocks one session; the next
// answer lets it stop with a message.
const maxBlocks = 3

type blockOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type messageOutput struct {
	SystemMessage string `json:"systemMessage"`
}

// stop blocks a sitting that would stop on a dirty tree (dirty-tree) or with
// nothing committed since SessionStart (no-commit, the relay's NoCommit stop
// reason). Without start.json the no-commit check is skipped and no block is
// counted; stop_hook_active bounds the loop either way. Outside a sitting it
// says nothing and writes nothing.
func stop(b []byte, env func(string) string, fx Effects) any {
	if !inSitting(env) {
		return nil
	}
	var in struct {
		SessionID      string `json:"session_id"`
		Cwd            string `json:"cwd"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	if json.Unmarshal(b, &in) != nil || in.StopHookActive || in.Cwd == "" {
		return nil
	}
	porcelain, err := fx.StatusPorcelain(in.Cwd)
	if err != nil {
		return nil
	}
	path := statePath(env, in.SessionID)
	st, haveState := loadState(fx, path)
	var reason string
	if n := countLines(porcelain); n > 0 {
		reason = fmt.Sprintf("dirty-tree: %d paths; commit the act or restore them, then stop", n)
	} else if haveState {
		head, err := fx.Head(in.Cwd)
		if err != nil || head != st.Head {
			return nil
		}
		reason = fmt.Sprintf("no-commit: nothing committed since %s; commit the act, then stop", st.Head)
	} else {
		return nil
	}
	if !haveState {
		return blockOutput{"block", "relay: " + reason}
	}
	if st.Blocks >= maxBlocks {
		return messageOutput{fmt.Sprintf("relay: stopping after %d nudges: %s", maxBlocks, reason)}
	}
	st.Blocks++
	_ = saveState(fx, path, st)
	return blockOutput{"block", "relay: " + reason}
}

func countLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}
