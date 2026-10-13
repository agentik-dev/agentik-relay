package hooks

import (
	"encoding/json"
	"path/filepath"
	"time"
	"unicode/utf8"
)

// packetCap is the most of the packet SessionStart injects.
const packetCap = 16384

type contextOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// HeadBeforeVar names the head a part's start records (AR68 ruling 2): the
// relay sets it to the stage's head_before for every part, so a continued
// part with nothing left to commit is not nudged no-commit.
const HeadBeforeVar = "RELAY_HEAD_BEFORE"

// sessionStart writes start.json in a sitting (on resume and compact an
// existing head, start time and block count are kept; else the head is
// HeadBeforeVar's when set, HEAD's when not) and, when RELAY_PACKET names
// a readable file, injects it after a one-line header in or out of a
// sitting. A state write that fails does not stop the injection.
func sessionStart(b []byte, env func(string) string, fx Effects) any {
	var in struct {
		SessionID string `json:"session_id"`
		Cwd       string `json:"cwd"`
		Source    string `json:"source"`
	}
	if json.Unmarshal(b, &in) != nil {
		return nil
	}
	packet := env("RELAY_PACKET")
	sitting := inSitting(env)
	if path := statePath(env, in.SessionID); sitting && path != "" {
		st := state{Cwd: in.Cwd, Started: fx.Now().UTC().Format(time.RFC3339), Packet: packet}
		old, ok := loadState(fx, path)
		if ok && old.Head != "" && (in.Source == "resume" || in.Source == "compact") {
			st.Head, st.Started, st.Blocks = old.Head, old.Started, old.Blocks
		} else if cut := env(HeadBeforeVar); cut != "" {
			st.Head = cut
		} else if in.Cwd != "" {
			st.Head, _ = fx.Head(in.Cwd)
		}
		_ = saveState(fx, path, st)
	}
	if packet == "" {
		return nil
	}
	file := packet
	if !filepath.IsAbs(file) && in.Cwd != "" {
		file = filepath.Join(in.Cwd, file)
	}
	body, err := fx.ReadFile(file)
	if err != nil {
		return nil
	}
	text := "relay: this sitting builds " + packet + "; commit per act; never push, amend or rebase; never a device"
	if sitting {
		text += "; the Stop guard blocks a dirty tree"
	}
	text += "\n"
	if len(body) > packetCap {
		cut := packetCap
		for cut > 0 && !utf8.RuneStart(body[cut]) {
			cut--
		}
		text += string(body[:cut]) + "\n[truncated]\n"
	} else {
		text += string(body)
	}
	return struct {
		HookSpecificOutput contextOutput `json:"hookSpecificOutput"`
	}{contextOutput{"SessionStart", text}}
}
