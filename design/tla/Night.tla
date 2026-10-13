------------------------------- MODULE Night -------------------------------
(***************************************************************************)
(* The night of design/relay-r1.md, modeled (§16): §1's sitting and night  *)
(* machines, §6's gate, §7's spawn and ceilings, §8's one measurement per  *)
(* root, and §15's bounds (the recursion guard, the breaker, the watchdog, *)
(* the sweep, the memo). Plain TLA+, no PlusCal. One action per row of     *)
(* §1's tables; the relay's own steps are one loop, NightStep, so its      *)
(* fairness is the relay's; the child, the load and the owner are the      *)
(* environment and get none.                                               *)
(***************************************************************************)
EXTENDS Naturals, FiniteSets

CONSTANTS Rows,         \* the queue's rows (model values)
          MaxProcs,     \* process slots; pid 1 is the relay
          MaxLoad,      \* the load both keys confirm, in units of cores: the
                        \* 1-minute load where the idle share of the CPU is at
                        \* or under the floor (AR47); a high load the idle
                        \* share does not confirm reads here as a low one
          OpenAt,       \* the breaker opens above this (4x the cores)
          CloseAt,      \* and closes under this (2x)
          MaxWait,      \* breaker polls before it trips (10 min / 30 s)
          StallAt,      \* watchdog polls without progress before the kill
          Wall,         \* watchdog polls before the wall clock kills
          MaxDepth,     \* the deepest RELAY_MEASURE_DEPTH a storm may reach
          Guard,        \* the recursion guard (AR10 act 0): a nested plan measures nothing
          RunGuard,     \* the run guard (AR11, C4): a nested `relay run` exits 2
          LinesMayPlan, \* a measured line may itself run `relay plan`
          PidRule,      \* who refuses at relay.pid (AR21 act 2): "any" live holder, or "ancestors" only
          LockKind      \* what the lock is (AR22 act 2): "port" (a listener, lost only with its holder) or "path" (a flock that follows relay.pid's name)

VARIABLES row,          \* r -> [st, reason, group]: §1's sitting state, its reason, the child's group
          night,        \* [st, reason, valid]: §1's night state
          procs,        \* pid -> the process in that slot
          measuring,    \* the pid holding night.LockRoot, 0 when free
          memo,         \* r -> the baseline is stored (night.Memo)
          tip, cut,     \* r -> the branch's tip and the commit it was cut at (0: no branch; 3: a late commit)
          mainMerges,   \* the rows main's first-parent history merged
          load,         \* the load, a bounded counter
          breaker,      \* "closed" | "open" | "tripped" (still open at MaxWait)
          waited,       \* the breaker polls spent this night
          stall,        \* r -> watchdog polls since the last progress
          clock,        \* r -> watchdog polls since the spawn
          checks,       \* r -> [base, gate, at, merged]: the baseline on main, the gate's rerun,
                        \* the tip a green gate measured (relay.Gate.Head) and the tip merged (0: none)
          anchorLoaded, \* r -> the baseline was measured above CloseAt
          written,      \* r -> [plan, spawn, exit, sitting]: the ledger, one boolean per line
          relaySt,      \* "up" | "dead" (killed past its trap) | "reaped" (the next start cleaned up)
          mainSha       \* main's sha, a counter: the relay's merge or a child's update-ref moves it (ADV10-1)

vars == <<row, night, procs, measuring, memo, tip, cut, mainMerges, load,
          breaker, waited, stall, clock, checks, anchorLoaded, written, relaySt, mainSha>>

Relay == 1
Pids == 1..MaxProcs
NoRow == "none"
MaxMain == Cardinality(Rows) + 1 \* the relay's merges and one child's update-ref
Terminal == {"landed", "green", "red", "failed", "skipped"}
AccountStops == {"usage-limit", "auth"} \* the account's stop (run 10 ruling 1)
Kinds == {"free", "relay", "measure", "plan", "child", "orphan", "escaped"}

Free == [kind |-> "free", parent |-> 0, group |-> 0, depth |-> 0, alive |-> FALSE,
         holdsStdout |-> FALSE, spawnedOpen |-> FALSE, row |-> NoRow, unlinked |-> FALSE]
Proc(k, par, g, d, r) ==
  [kind |-> k, parent |-> par, group |-> g, depth |-> d, alive |-> TRUE,
   holdsStdout |-> FALSE, spawnedOpen |-> FALSE, row |-> r, unlinked |-> FALSE]

\* POSIX: a pid is never reused while a process group of that id lives.
\* A pid is never reused while it names a live group or, for an escaped
\* process, the session it left (its `parent`, the line's session id).
FreePids == {p \in Pids : procs[p].kind = "free" /\ ~\E q \in Pids :
                procs[q].alive /\ (procs[q].group = p \/ (procs[q].kind = "escaped" /\ procs[q].parent = p))}
Lowest(S) == CHOOSE p \in S : \A q \in S : p <= q
KillGroup(g) == [q \in Pids |-> IF procs[q].alive /\ procs[q].group = g THEN Free ELSE procs[q]]
\* The session sweep of line p (effects.SweepSession, AR17C): its group, and
\* every escaped process whose session was the line's.
KillSession(p) == [q \in Pids |-> IF procs[q].alive /\ (procs[q].group = p \/ (procs[q].kind = "escaped" /\ procs[q].parent = p))
                                  THEN Free ELSE procs[q]]
Escaped(p) == \E q \in Pids : procs[q].alive /\ procs[q].kind = "escaped" /\ procs[q].parent = p
ChildOf(r) == {p \in Pids : procs[p].alive /\ procs[p].kind = "child"
                            /\ procs[p].parent = Relay /\ procs[p].row = r}
\* The sweep's end for row r (effects.Sweep, then effects.SweepEscaped over
\* the descendants the watchdog's polls saw, ADV6-8): the group dead, and
\* those that left it when a poll saw them (seen); one no poll saw lives on
\* but holds nothing: its stream was closed from the read end PipeGrace
\* after the exit (ADV6-7). Held is that cut: the row failed `orphans`.
Swept(r, seen) == [q \in Pids |-> IF procs[q].alive /\ procs[q].row = r
                                     /\ (procs[q].group = row[r].group \/ seen)
                                    THEN Free
                                  ELSE IF procs[q].row = r THEN [procs[q] EXCEPT !.holdsStdout = FALSE]
                                  ELSE procs[q]]
Held(r, seen) == \E q \in Pids : procs[q].alive /\ procs[q].row = r /\ procs[q].holdsStdout
                                /\ procs[q].group # row[r].group /\ ~seen
\* The relay's read ends close with it: nothing holds a stream any more.
Unheld(ps) == [q \in Pids |-> [ps[q] EXCEPT !.holdsStdout = FALSE]]
\* --parallel 1, the default (§8): one sitting at a time.
Busy == \E r \in Rows : row[r].st \in {"running", "exited", "swept"}
Waiting == \/ /\ night.st = "planning" /\ night.valid /\ measuring = 0
              /\ \E r \in Rows : row[r].st = "ready" /\ checks[r].base = "none"
           \/ night.st = "running" /\ ~Busy /\ \E r \in Rows : row[r].st = "planned"
\* The load is read only by a breaker and a measured line; it moves only
\* while one of them can read it.
Read == Waiting \/ measuring # 0 \/ breaker # "closed"

TypeOK ==
  /\ row \in [Rows -> [st : {"ready", "planned", "running", "exited", "swept"} \cup Terminal,
                       reason : STRING, group : 0..MaxProcs]]
  /\ night \in [st : {"planning", "running", "draining", "done"}, reason : STRING, valid : BOOLEAN]
  /\ procs \in [Pids -> [kind : Kinds, parent : 0..MaxProcs, group : 0..MaxProcs,
                         depth : 0..MaxDepth, alive : BOOLEAN, holdsStdout : BOOLEAN,
                         spawnedOpen : BOOLEAN, row : Rows \cup {NoRow}, unlinked : BOOLEAN]]
  /\ measuring \in 0..MaxProcs
  /\ memo \in [Rows -> BOOLEAN] /\ anchorLoaded \in [Rows -> BOOLEAN]
  /\ tip \in [Rows -> 0..3] /\ cut \in [Rows -> 0..1] /\ mainMerges \subseteq Rows
  /\ load \in 0..MaxLoad /\ breaker \in {"closed", "open", "tripped"} /\ waited \in 0..MaxWait
  /\ stall \in [Rows -> 0..StallAt] /\ clock \in [Rows -> 0..Wall]
  /\ checks \in [Rows -> [base : {"none", "green", "red", "unrun"},
                          gate : {"none", "green", "red", "unrun", "same-as-main"},
                          at : 0..3, merged : 0..3, main : 0..MaxMain, onto : 0..MaxMain]]
  /\ mainSha \in 0..MaxMain
  /\ written \in [Rows -> [plan : BOOLEAN, spawn : BOOLEAN, exit : BOOLEAN, sitting : BOOLEAN]]
  /\ relaySt \in {"up", "dead", "reaped"}

\* A row's branch at the night's start: none, fresh (cut, not merged), or
\* merged by an earlier night (moved and on main's first-parent line).
Init ==
  /\ \E b \in [Rows -> {"none", "fresh", "merged"}] :
       /\ mainMerges = {r \in Rows : b[r] = "merged"}
       /\ cut = [r \in Rows |-> IF b[r] = "none" THEN 0 ELSE 1]
       /\ tip = [r \in Rows |-> CASE b[r] = "none" -> 0 [] b[r] = "fresh" -> 1 [] OTHER -> 2]
  /\ row = [r \in Rows |-> [st |-> "ready", reason |-> "none", group |-> 0]]
  /\ night = [st |-> "planning", reason |-> "none", valid |-> FALSE]
  /\ procs = [p \in Pids |-> IF p = Relay
                             THEN [Free EXCEPT !.kind = "relay", !.group = Relay, !.alive = TRUE]
                             ELSE Free]
  /\ measuring = 0
  /\ memo = [r \in Rows |-> FALSE]
  /\ load = 0 /\ breaker = "closed" /\ waited = 0
  /\ stall = [r \in Rows |-> 0] /\ clock = [r \in Rows |-> 0]
  /\ checks = [r \in Rows |-> [base |-> "none", gate |-> "none", at |-> 0, merged |-> 0,
                               main |-> 0, onto |-> 0]]
  /\ mainSha = 0
  /\ anchorLoaded = [r \in Rows |-> FALSE]
  /\ written = [r \in Rows |-> [plan |-> FALSE, spawn |-> FALSE, exit |-> FALSE, sitting |-> FALSE]]
  /\ relaySt = "up"

---------------------------------------------------------------------------
(* The plan (§15 fail early, scope, memo, bounded). *)

\* ready -> invalid -> skipped, or ready -> landed. Landed reads main's
\* first-parent merges (the design of record; Go's night.landed, AR13 act 8).
PlanValidate ==
  /\ night.st = "planning" /\ ~night.valid
  /\ IF \E r \in Rows : cut[r] # 0 /\ r \notin mainMerges   \* a branch exists (§8)
       THEN /\ night' = [st |-> "done", reason |-> "invalid", valid |-> FALSE]
            /\ row' = [r \in Rows |-> [row[r] EXCEPT !.st = "skipped", !.reason = "invalid"]]
            /\ written' = [r \in Rows |-> [written[r] EXCEPT !.plan = TRUE]]
       ELSE /\ night' = [night EXCEPT !.valid = TRUE]
            /\ row' = [r \in Rows |-> IF r \in mainMerges
                                      THEN [row[r] EXCEPT !.st = "landed", !.reason = "landed"]
                                      ELSE row[r]]
            /\ UNCHANGED written
  /\ UNCHANGED <<mainSha, relaySt, procs, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded>>

\* The relay (depth 0) runs one baseline line on main, its shell at depth 1,
\* under the root lock and a closed breaker.
Measure(r) ==
  /\ night.st = "planning" /\ night.valid
  /\ row[r].st = "ready" /\ checks[r].base = "none"
  /\ measuring = 0
  /\ breaker = "closed" /\ load <= OpenAt
  /\ FreePids # {}
  /\ LET p == Lowest(FreePids)
     IN /\ procs' = [procs EXCEPT ![p] = Proc("measure", Relay, p, 1, r)]
        /\ measuring' = p
  /\ UNCHANGED <<mainSha, relaySt, row, night, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* The line ends; its group is swept; a red read above CloseAt is unrun:
\* loaded (D4) and nothing loaded is stored. ready -> plan -> planned.
MeasureDone(r) ==
  \E p \in Pids :
    /\ procs[p].alive /\ procs[p].kind = "measure" /\ procs[p].parent = Relay /\ procs[p].row = r
    /\ \E res \in {"green", "red"} :
         LET loaded == load > CloseAt
         IN /\ checks' = [checks EXCEPT ![r].base = IF (loaded /\ res = "red") \/ Escaped(p) THEN "unrun" ELSE res]
            /\ anchorLoaded' = [anchorLoaded EXCEPT ![r] = loaded]
            /\ memo' = [memo EXCEPT ![r] = ~loaded]
    /\ procs' = KillSession(p)
    /\ measuring' = 0
    /\ row' = IF row[r].st = "ready" THEN [row EXCEPT ![r].st = "planned"] ELSE row
    /\ written' = [written EXCEPT ![r].plan = TRUE]
    /\ UNCHANGED <<mainSha, relaySt, night, tip, cut, mainMerges, load, breaker, waited, stall, clock>>

\* A measured line runs `relay plan` (a test that builds the binary, a make
\* target). The plan inherits the line's depth. Guarded, it measures nothing
\* and ends with its line; unguarded and missing the memo (the line being
\* measured is not stored yet), it measures again one level down, in its own
\* group, outside the parent's in-process lock: the storm.
LineStartsPlan(p) ==
  /\ LinesMayPlan
  /\ procs[p].alive /\ procs[p].kind = "measure" /\ procs[p].depth < MaxDepth
  /\ ~\E q \in Pids : procs[q].alive /\ procs[q].parent = p
  /\ FreePids # {}
  /\ LET q == Lowest(FreePids)
         plan == Proc("plan", p, procs[p].group, procs[p].depth, procs[p].row)
     IN IF Guard \/ memo[procs[p].row]
          THEN procs' = [procs EXCEPT ![q] = plan]
          ELSE /\ Cardinality(FreePids) >= 2
               /\ LET m == Lowest(FreePids \ {q})
                  IN procs' = [procs EXCEPT ![q] = plan,
                                            ![m] = Proc("measure", q, m, procs[p].depth + 1, procs[p].row)]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* A measured line leaves a daemon (ADV10-12): it forks, calls setsid and
\* closes its stdio, so it is in no group of the relay's and holds no
\* stream; `parent` keeps the session it left. TLC's bound: the relay's own
\* line (depth 1) of one row, one escapee at a time, before the line plans.
LineEscapes(p) ==
  /\ procs[p].alive /\ procs[p].kind = "measure" /\ procs[p].parent = Relay
  /\ procs[p].row = CHOOSE r \in Rows : TRUE
  /\ ~\E q \in Pids : procs[q].alive /\ (procs[q].kind = "escaped" \/ procs[q].parent = p)
  /\ FreePids # {}
  /\ LET e == Lowest(FreePids)
     IN procs' = [procs EXCEPT ![e] = Proc("escaped", p, e, procs[p].depth, procs[p].row)]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

---------------------------------------------------------------------------
(* The sitting (§1, §7, §15 watchdog and sweep). *)

\* planned -> stage -> staged -> spawn -> running: the worktree is cut, the
\* child spawned in its own group at depth 1, holding the stream's stdout,
\* only through a closed breaker.
Spawn(r) ==
  /\ night.st = "running"
  /\ row[r].st = "planned" /\ ~Busy
  /\ breaker = "closed" /\ load <= OpenAt
  /\ FreePids # {}
  /\ LET p == Lowest(FreePids)
     IN /\ procs' = [procs EXCEPT ![p] = [Proc("child", Relay, p, 1, r) EXCEPT
                                           !.holdsStdout = TRUE, !.spawnedOpen = (breaker # "closed")]]
        /\ row' = [row EXCEPT ![r].st = "running", ![r].group = p]
  /\ cut' = [cut EXCEPT ![r] = 1] /\ tip' = [tip EXCEPT ![r] = 1]
  /\ stall' = [stall EXCEPT ![r] = 0] /\ clock' = [clock EXCEPT ![r] = 0]
  /\ written' = [written EXCEPT ![r].spawn = TRUE]
  /\ checks' = [checks EXCEPT ![r].main = mainSha] \* Staged.BaseSha
  /\ UNCHANGED <<mainSha, relaySt, night, measuring, memo, mainMerges, load, breaker, waited,
                 anchorLoaded>>

\* The child runs `relay run` (C4, ADV6-1). Under RunGuard the verb exits 2
\* before any stage and the step is refused; without it a nested relay in the
\* child's group spawns its own child.
NestedRun(c) ==
  /\ ~RunGuard
  /\ procs[c].alive /\ procs[c].kind = "child"
  /\ Cardinality(FreePids) >= 2
  /\ LET n == Lowest(FreePids)
         m == Lowest(FreePids \ {n})
     IN procs' = [procs EXCEPT
                    ![n] = Proc("relay", c, procs[c].group, procs[c].depth, procs[c].row),
                    ![m] = [Proc("child", n, m, procs[c].depth, procs[c].row) EXCEPT !.holdsStdout = TRUE]]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* relay.pid's holders: the night's relay while it is up, and any other live
\* relay that got past its guards. A dead holder's flock dies with it.
Holders == {h \in Pids : procs[h].alive /\ procs[h].kind = "relay" /\ (h = Relay => relaySt = "up")}

\* The pids on p's parent chain, p first (bounded by the slots).
RECURSIVE Above(_, _)
Above(p, n) == IF p = 0 \/ n = 0 THEN {} ELSE {p} \cup Above(procs[p].parent, n - 1)

\* A relay whose parent is par may start (night.PidFile, AR21 act 2,
\* ADV12-3). "any": every live holder refuses, since its exclusive flock
\* fails the next comer's LOCK_NB. "ancestors", the rule before AR21: only a
\* holder on the comer's parent chain refuses.
\* The holders a comer can see (AR22 act 2, ADV13-4). "port": every live
\* holder, since the kernel keeps its listener whatever a sitting does to
\* relay.pid. "path": a holder whose pid file's name was unlinked is not
\* seen, since the comer makes a new file there and flocks that.
Seen == IF LockKind = "port" THEN Holders ELSE {h \in Holders : ~procs[h].unlinked}
PidGuard(par) == IF PidRule = "any" THEN Seen = {}
                 ELSE Seen \cap Above(par, MaxProcs) = {}

\* The child starts `relay run` detached with its env stripped (ADV12-3:
\* `env -i`, `setsid`, a launch agent): no RELAY_ROW and no depth, so
\* RunGuard and the hook see nothing, and its parent is launchd, never the
\* night's relay. Only PidGuard stands; past it the second relay spawns its
\* own child. TLC's bound: while the night's relay is up (a relay started
\* after its death is the next start, Reap's step).
DetachedRun(c) ==
  /\ relaySt = "up"
  /\ procs[c].alive /\ procs[c].kind = "child"
  /\ PidGuard(0)
  /\ Cardinality(FreePids) >= 2
  /\ LET n == Lowest(FreePids)
         m == Lowest(FreePids \ {n})
     IN procs' = [procs EXCEPT ![n] = Proc("relay", 0, n, 1, procs[c].row),
                               ![m] = Proc("child", n, m, 1, procs[c].row)]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* A sitting removes relay.pid's name (ADV13-4: `rm`, `mv`, its dir
\* renamed); every live holder's name is gone.
Unlink(c) ==
  /\ procs[c].alive /\ procs[c].kind = "child"
  /\ \E h \in Holders : ~procs[h].unlinked
  /\ procs' = [q \in Pids |-> IF q \in Holders THEN [procs[q] EXCEPT !.unlinked = TRUE] ELSE procs[q]]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* A holder other than the night's relay dies (the night's is RelayDies):
\* the kernel frees its lock with it, port or flock.
HolderDies(h) ==
  /\ h # Relay /\ h \in Holders
  /\ procs' = [procs EXCEPT ![h] = Free]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* running -> stream -> running: a commit, or a background process, in the
\* child's group (swept), or one that left it holding stdout (ADV6-7).
Progress(r) ==
  /\ row[r].st = "running"
  /\ \E c \in ChildOf(r) :
       \/ /\ tip' = [tip EXCEPT ![r] = 2]
          /\ UNCHANGED procs
       \/ /\ FreePids # {}
          /\ ~\E q \in Pids : procs[q].alive /\ procs[q].kind = "orphan" /\ procs[q].row = r
          \* A descendant that left the group holds the stream only while the
          \* relay's read end lives: forked after RelayDies, it inherits a pipe
          \* with no reader, which holds nothing (Unheld; ar16: TLC's first run
          \* found this model let it hold one, not a path in Go).
          /\ LET o == Lowest(FreePids)
             IN \E left \in BOOLEAN :
                  procs' = [procs EXCEPT ![o] = [Proc("orphan", c, IF left THEN o ELSE procs[c].group,
                                                      procs[c].depth, r)
                                                 EXCEPT !.holdsStdout = left /\ relaySt = "up"]]
          /\ UNCHANGED tip
  /\ stall' = [stall EXCEPT ![r] = 0]
  /\ UNCHANGED <<mainSha, relaySt, row, night, measuring, memo, cut, mainMerges, load, breaker, waited,
                 clock, checks, anchorLoaded, written>>

\* The watchdog's sample (night.WatchdogPoll): the wall and the stall move.
Poll(r) ==
  /\ relaySt = "up"
  /\ row[r].st = "running"
  /\ clock[r] < Wall
  /\ clock' = [clock EXCEPT ![r] = clock[r] + 1]
  /\ stall' = [stall EXCEPT ![r] = IF stall[r] < StallAt THEN stall[r] + 1 ELSE stall[r]]
  /\ UNCHANGED <<mainSha, relaySt, row, night, procs, measuring, memo, tip, cut, mainMerges, load, breaker,
                 waited, checks, anchorLoaded, written>>

\* Stalled: the group killed at once, the row red `stalled` (a packet
\* failure); RunRow's Wait, sweep and cut follow, so a held stream fails it
\* `orphans` (Swept, Held).
Stall(r) ==
  /\ relaySt = "up"
  /\ row[r].st = "running" /\ stall[r] = StallAt
  /\ \E seen \in BOOLEAN :
       /\ procs' = Swept(r, seen)
       /\ row' = IF Held(r, seen) THEN [row EXCEPT ![r].st = "failed", ![r].reason = "orphans"]
                                  ELSE [row EXCEPT ![r].st = "red", ![r].reason = "stalled"]
  /\ written' = [written EXCEPT ![r].exit = TRUE, ![r].sitting = TRUE]
  /\ stall' = [stall EXCEPT ![r] = 0] /\ clock' = [clock EXCEPT ![r] = 0]
  /\ UNCHANGED <<mainSha, relaySt, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 checks, anchorLoaded>>

\* running -> ceiling-hit: the wall (SIGTERM, KillGrace, SIGKILL) ends the
\* child; the sweep and the read end's close follow (Sweep).
WallExpires(r) ==
  /\ row[r].st = "running" /\ clock[r] = Wall
  /\ procs' = [q \in Pids |-> IF q \in ChildOf(r) THEN Free ELSE procs[q]]
  /\ row' = [row EXCEPT ![r].st = "exited", ![r].reason = "wall-clock"]
  /\ written' = [written EXCEPT ![r].exit = TRUE]
  /\ UNCHANGED <<mainSha, relaySt, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded>>

\* running -> exit -> exited: the Wait returns at the child's exit whoever
\* still holds stdout (ADV6-7: a holder no longer stalls the sitting; the
\* sweep and the read end's close deal with it). The exit line is the
\* relay's: a dead relay's child lives on until it is reaped.
ChildExit(r) ==
  /\ relaySt = "up"
  /\ row[r].st = "running"
  /\ \E c \in ChildOf(r) : procs' = [procs EXCEPT ![c] = Free]
  /\ row' = [row EXCEPT ![r].st = "exited"]
  /\ written' = [written EXCEPT ![r].exit = TRUE]
  /\ UNCHANGED <<mainSha, relaySt, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded>>

\* running -> exit -> exited, the account's (run 10 ruling 1, AR17D): the
\* child's result names a usage limit or a login (stream.Account), or a
\* refused rate_limit_event has the relay kill it.
AccountFails(r) ==
  /\ relaySt = "up"
  /\ row[r].st = "running"
  /\ \E c \in ChildOf(r) : procs' = [procs EXCEPT ![c] = Free]
  /\ \E why \in AccountStops : row' = [row EXCEPT ![r].st = "exited", ![r].reason = why]
  /\ written' = [written EXCEPT ![r].exit = TRUE]
  /\ UNCHANGED <<mainSha, relaySt, night, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded>>

\* After the Wait, the group's survivors are killed (effects.Sweep), then
\* the escaped descendants a poll saw (effects.SweepEscaped); SIGKILL ends
\* every one (a survivor of three kills is Go's `orphans` stop). A stream
\* still held after that is cut, the row failed `orphans` (Held). A wall
\* kill fails here. An account stop fails here with its reason, never the
\* gate, and in the same step (Go's collect) stops the night: draining, so
\* no Spawn; the other running rows' kill is Go's (the model runs one
\* sitting at a time; TestAR17D_0_ParallelRowsKilled).
Sweep(r) ==
  /\ row[r].st = "exited"
  /\ \E seen \in BOOLEAN :
       /\ procs' = Swept(r, seen)
       /\ IF row[r].reason \in AccountStops
            THEN /\ row' = [row EXCEPT ![r].st = "failed"]
                 /\ written' = [written EXCEPT ![r].sitting = TRUE]
            ELSE IF row[r].reason = "wall-clock" \/ Held(r, seen)
            THEN /\ row' = [row EXCEPT ![r].st = "failed",
                                       ![r].reason = IF Held(r, seen) THEN "orphans" ELSE "wall-clock"]
                 /\ written' = [written EXCEPT ![r].sitting = TRUE]
            ELSE /\ row' = [row EXCEPT ![r].st = "swept"]
                 /\ UNCHANGED written
  /\ night' = IF row[r].reason \in AccountStops /\ night.st \in {"running", "draining"}
                THEN [night EXCEPT !.st = "draining", !.reason = row[r].reason]
                ELSE night
  /\ stall' = [stall EXCEPT ![r] = 0] /\ clock' = [clock EXCEPT ![r] = 0]
  /\ UNCHANGED <<mainSha, relaySt, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 checks, anchorLoaded>>

\* exited -> gate-green | gate-red | gate-failed | gate-unrun (§6). A failing
\* rerun is same-as-main only against a red baseline, which is never loaded.
\* The gate binds the branch's tip first (ADV6-3): a pass records it.
Gate(r) ==
  /\ row[r].st = "swept"
  /\ written' = [written EXCEPT ![r].sitting = TRUE]
  /\ IF tip[r] = cut[r]
       THEN /\ row' = [row EXCEPT ![r].st = "failed", ![r].reason = "no-commit"]
            /\ UNCHANGED checks
       ELSE IF mainSha # checks[r].main \* ADV10-1: the base moved, not by the relay's merge
       THEN /\ checks' = [checks EXCEPT ![r].gate = "red"]
            /\ row' = [row EXCEPT ![r].st = "red", ![r].reason = "main-moved"]
       ELSE \E res \in {"pass", "fail", "unrun", "dirty"} :
              CASE res = "pass" ->
                     /\ checks' = [checks EXCEPT ![r].gate = "green", ![r].at = tip[r]]
                     /\ row' = [row EXCEPT ![r].st = "green"]
                [] res = "fail" /\ checks[r].base = "red" ->
                     /\ checks' = [checks EXCEPT ![r].gate = "same-as-main", ![r].at = tip[r]]
                     /\ row' = [row EXCEPT ![r].st = "green"]
                [] res = "fail" ->
                     /\ checks' = [checks EXCEPT ![r].gate = "red"]
                     /\ row' = [row EXCEPT ![r].st = "red", ![r].reason = "gate-red"]
                [] res = "unrun" ->
                     /\ checks' = [checks EXCEPT ![r].gate = "unrun"]
                     /\ row' = [row EXCEPT ![r].st = "red", ![r].reason = "gate-unrun"]
                [] OTHER ->
                     /\ UNCHANGED checks
                     /\ row' = [row EXCEPT ![r].st = "failed", ![r].reason = "dirty-tree"]
  /\ UNCHANGED <<mainSha, relaySt, night, procs, measuring, memo, tip, cut, mainMerges, load, breaker,
                 waited, stall, clock, anchorLoaded>>

\* merge-on-green: a first-parent merge of the moved branch into main, of
\* the sha the gate measured; a tip that moved after the gate is refused
\* (gate.MergeIfGreen's `moved`, ADV6-3). The merge's verify passes green
\* alone (ADV6-5): a row green as main is red stays branch-only.
Merge(r) ==
  /\ row[r].st = "green" /\ r \notin mainMerges
  /\ night.st \in {"running", "draining"}
  /\ checks[r].gate = "green"
  /\ tip[r] = checks[r].at
  /\ mainSha = checks[r].main /\ mainSha < MaxMain \* gate.MergeIfGreen's main-moved (ADV10-1)
  /\ mainMerges' = mainMerges \cup {r}
  /\ mainSha' = mainSha + 1
  \* The relay's own merge moves every other row's expected main with it
  \* (gate.expectedBase: the last `merged` line after the row's stage).
  /\ checks' = [s \in Rows |->
                  IF s = r THEN [checks[r] EXCEPT !.merged = tip[r], !.onto = mainSha]
                  ELSE IF checks[s].main = mainSha /\ checks[s].merged = 0
                       THEN [checks[s] EXCEPT !.main = mainSha + 1] ELSE checks[s]]
  /\ UNCHANGED <<relaySt, row, night, procs, measuring, memo, tip, cut, load, breaker, waited,
                 stall, clock, anchorLoaded, written>>

\* A running child moves main from its linked worktree (`git update-ref
\* refs/heads/main`, ADV10-1): nothing in the tree stops it; the gate must.
MoveMain(r) ==
  /\ row[r].st = "running" /\ mainSha < MaxMain
  /\ \A s \in Rows : checks[s].main = mainSha \* once a night: the model's bound
  /\ mainSha' = mainSha + 1
  /\ UNCHANGED <<relaySt, row, night, procs, measuring, memo, tip, cut, mainMerges, load, breaker,
                 waited, stall, clock, checks, anchorLoaded, written>>

\* A process of the row still alive after its gate (one that left the group,
\* ADV6-7) commits on the branch: the tip moves past the sha the gate measured.
LateCommit(r) ==
  /\ row[r].st = "green" /\ r \notin mainMerges /\ tip[r] # 3
  /\ \E q \in Pids : procs[q].alive /\ procs[q].row = r
  /\ tip' = [tip EXCEPT ![r] = 3]
  /\ UNCHANGED <<mainSha, relaySt, row, night, procs, measuring, memo, cut, mainMerges, load, breaker,
                 waited, stall, clock, checks, anchorLoaded, written>>

---------------------------------------------------------------------------
(* The load and the breaker (§15). *)

\* The load rises only while the breaker is closed: LoadFair's assumption is
\* that a load the breaker waits on eventually falls.
LoadUp ==
  /\ Read /\ breaker = "closed" /\ load < MaxLoad
  /\ load' = load + 1
  /\ UNCHANGED <<mainSha, relaySt, row, night, procs, measuring, memo, tip, cut, mainMerges, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

LoadDown ==
  /\ Read /\ load > 0
  /\ load' = load - 1
  /\ UNCHANGED <<mainSha, relaySt, row, night, procs, measuring, memo, tip, cut, mainMerges, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* Still open after the night's MaxWait polls: the waiter fails `load`,
\* the night drains `stopped:load`.
BreakerTrip ==
  /\ breaker = "open" /\ waited = MaxWait
  /\ breaker' = "tripped"
  /\ night' = IF night.st \in {"planning", "running"}
                THEN [night EXCEPT !.st = "draining", !.reason = "load"]
                ELSE night
  /\ UNCHANGED <<mainSha, relaySt, row, procs, measuring, memo, tip, cut, mainMerges, load, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* The waiter's read, the relay's own step: above OpenAt it opens (else the
\* waiter proceeds: Measure, Spawn).
BreakerOpen ==
  /\ breaker = "closed" /\ load > OpenAt /\ Waiting
  /\ breaker' = "open"
  /\ UNCHANGED <<mainSha, relaySt, row, night, procs, measuring, memo, tip, cut, mainMerges, load, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* Open, each poll spends one of the night's MaxWait and closes under
\* CloseAt; at MaxWait the poll is the trip; tripped, a read under CloseAt
\* closes it.
BreakerReads ==
  \/ /\ breaker = "open" /\ waited < MaxWait
     /\ waited' = waited + 1
     /\ breaker' = IF load < CloseAt THEN "closed" ELSE "open"
     /\ UNCHANGED night
  \/ /\ breaker = "tripped" /\ load < CloseAt
     /\ breaker' = "closed"
     /\ UNCHANGED <<mainSha, relaySt, night, waited>>
  \/ BreakerTrip

BreakerPoll ==
  /\ relaySt = "up"
  /\ BreakerReads
  /\ UNCHANGED <<mainSha, relaySt, row, procs, measuring, memo, tip, cut, mainMerges, load,
                 stall, clock, checks, anchorLoaded, written>>

---------------------------------------------------------------------------
(* The night (§1). *)

OwnerStop ==
  /\ night.st \in {"planning", "running"}
  /\ night' = [night EXCEPT !.st = "draining", !.reason = "owner-stop"]
  /\ UNCHANGED <<mainSha, relaySt, row, procs, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

Failed == {r \in Rows : row[r].st = "failed"}

\* The night machine: planning -> running; a sitting failure drains it; a
\* draining night halts every waiting row (halt -> skipped); every row
\* terminal, it stops with its reason.
NightMove ==
  \/ /\ night.st = "planning" /\ night.valid
     /\ \A r \in Rows : row[r].st # "ready"
     /\ night' = [night EXCEPT !.st = "running"]
     /\ UNCHANGED <<mainSha, relaySt, row, written>>
  \/ /\ night.st = "running" /\ Failed # {}
     /\ night' = [night EXCEPT !.st = "draining", !.reason = row[CHOOSE r \in Failed : TRUE].reason]
     /\ UNCHANGED <<mainSha, relaySt, row, written>>
  \/ /\ night.st = "draining"
     /\ \E r \in Rows : row[r].st \in {"ready", "planned"}
     /\ row' = [r \in Rows |-> IF row[r].st \in {"ready", "planned"}
                               THEN [row[r] EXCEPT !.st = "skipped", !.reason = night.reason]
                               ELSE row[r]]
     /\ written' = [r \in Rows |-> IF row[r].st \in {"ready", "planned"}
                                   THEN [written[r] EXCEPT !.sitting = TRUE]
                                   ELSE written[r]]
     /\ UNCHANGED night
  \/ /\ \/ night.st = "draining"
        \/ night.st = "running" /\ Failed = {}
     /\ \A r \in Rows : row[r].st \in Terminal
     /\ night' = [night EXCEPT !.st = "done",
                               !.reason = IF night.st = "running" THEN "queue-empty" ELSE night.reason]
     /\ UNCHANGED <<mainSha, relaySt, row, written>>

NightMoveStep ==
  /\ NightMove
  /\ UNCHANGED <<mainSha, relaySt, procs, measuring, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded>>

\* The relay's loop: one step of whatever the relay owes next.
NightStep ==
  /\ relaySt = "up"
  /\ \/ PlanValidate
     \/ NightMoveStep
     \/ BreakerOpen
     \/ \E r \in Rows : \/ Measure(r) \/ MeasureDone(r) \/ Spawn(r) \/ WallExpires(r)
                        \/ Sweep(r) \/ Gate(r) \/ Merge(r)

---------------------------------------------------------------------------
(* The dead relay (ADV6-2). Every group the relay starts, a line's or a   *)
(* child's, is in <night>/groups.json from its start to its end.          *)

\* The groups a killed relay leaves recorded: every row's child group and
\* the measured line's.
Recorded == ({row[r].group : r \in Rows} \cup {measuring}) \ {0}
KillRecorded == [q \in Pids |-> IF q # Relay /\ procs[q].alive /\ procs[q].group \in Recorded
                               THEN Free ELSE procs[q]]

\* INT, TERM or HUP, trapped once (night.Trap from main, before any
\* measurement, AR17C; night.Run's for the night): every recorded group gets
\* SIGTERM, KillGrace, SIGKILL, and the measured line's session is swept
\* before the exit (Drain); every row not yet terminal writes its sitting
\* line (failed or skipped, owner-kill); the night stops owner-kill.
Signal ==
  /\ relaySt = "up" /\ night.st # "done"
  /\ procs' = Unheld([q \in Pids |-> IF measuring # 0 /\ procs[q].alive /\ procs[q].kind = "escaped"
                                         /\ procs[q].parent = measuring THEN Free ELSE KillRecorded[q]])
  /\ measuring' = 0
  /\ row' = [r \in Rows |->
               CASE row[r].st \in {"running", "exited", "swept"} ->
                      [row[r] EXCEPT !.st = "failed", !.reason = "owner-kill"]
                 [] row[r].st \in {"ready", "planned"} ->
                      [row[r] EXCEPT !.st = "skipped", !.reason = "owner-kill"]
                 [] OTHER -> row[r]]
  /\ written' = [r \in Rows |-> IF row[r].st \in Terminal THEN written[r]
                               ELSE [written[r] EXCEPT !.sitting = TRUE,
                                                       !.exit = written[r].exit \/ row[r].st = "running"]]
  /\ night' = [night EXCEPT !.st = "done", !.reason = "owner-kill"]
  /\ breaker' = "closed"
  /\ stall' = [r \in Rows |-> 0] /\ clock' = [r \in Rows |-> 0]
  /\ UNCHANGED <<mainSha, relaySt, memo, tip, cut, mainMerges, load, waited, checks, anchorLoaded>>

\* A kill the trap never sees (SIGKILL, a power cut): the relay is gone,
\* its children and its line run on unsupervised, no sitting line is
\* written, groups.json stays.
RelayDies ==
  /\ relaySt = "up" /\ night.st # "done"
  /\ relaySt' = "dead"
  /\ night' = [night EXCEPT !.st = "done", !.reason = "relay-dead"]
  /\ breaker' = "closed"
  /\ procs' = Unheld(procs)
  /\ UNCHANGED <<mainSha, row, measuring, memo, tip, cut, mainMerges, load, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* The next start (night.Reap): every recorded group whose leader still has
\* its recorded start time is killed with a `reaped` line; the rest (a pid
\* reused, which FreePids keeps apart here) are dropped with the file.
Reap ==
  /\ relaySt = "dead"
  /\ relaySt' = "reaped"
  /\ procs' = KillRecorded
  /\ measuring' = 0
  /\ UNCHANGED <<mainSha, row, night, memo, tip, cut, mainMerges, load, breaker, waited,
                 stall, clock, checks, anchorLoaded, written>>

\* A stopped night stutters.
Done == night.st = "done" /\ UNCHANGED vars

Next ==
  \/ NightStep
  \/ BreakerPoll
  \/ LoadUp \/ LoadDown \/ OwnerStop
  \/ Signal \/ RelayDies \/ Reap
  \/ \E p \in Pids : LineStartsPlan(p) \/ LineEscapes(p) \/ NestedRun(p) \/ DetachedRun(p)
     \/ Unlink(p) \/ HolderDies(p)
  \/ \E r \in Rows : Progress(r) \/ Poll(r) \/ Stall(r) \/ ChildExit(r) \/ AccountFails(r) \/ LateCommit(r) \/ MoveMain(r)
  \/ Done

Spec == Init /\ [][Next]_vars

---------------------------------------------------------------------------
(* Safety: the ten invariants (§16). Each names its Go guard and test (§11).  *)

Alive(k) == {p \in Pids : procs[p].alive /\ procs[p].kind = k}

\* Guard: effects.Nested in BuildPlan and BaselineOnMain (OSShell.Run sets
\* RELAY_MEASURE_DEPTH=1). Tests: TestAR10_0_PlanNestedMeasuresNothing,
\* TestAR10_0_NestedPlanMeasuresOnce.
NoNestedMeasure == \A p \in Alive("measure") : procs[p].depth = 1 /\ procs[p].parent = Relay

\* Guard: night.LockRoot (in-process; one relay per root). Test:
\* TestAR10_4_OneMeasurementPerRoot.
OneMeasurePerRoot == Cardinality(Alive("measure")) <= 1

\* Guard: night.Breaker.Wait before RunRow's spawn line. Test:
\* TestAR10_5_StillOpenSpawnStopsLoad.
NoSpawnWhenOpen == \A p \in Alive("child") : ~procs[p].spawnedOpen

\* Guard: effects.Sweep in RunRow after the Wait. Tests: TestAR10_8_OrphansRed,
\* TestAR10_8_SweepRounds. A process that left the group is the sweep's too
\* when a watchdog poll saw it (effects.SweepEscaped, ADV6-8); one that holds
\* the stream is an orphan the sweep must account for: its stream closed
\* from the read end, the row failed `orphans` (ADV6-7). Tests:
\* TestAR13_3_HeldStreamIsOrphans, TestAR13_3_DescendantsSwept.
\* A dead relay writes no sitting line: once the next start has reaped, the
\* row's group is gone too (ADV6-2; night.Run's trap and night.Reap. Tests:
\* TestAR13_2_SignalReapsGroups, TestAR13_2_NextStartReaps,
\* TestAR13_2_ReusedPidNotKilled).
\* The measuring half (AR17C): while the relay is up, an escaped process
\* lives only beside the line it left, which is still measuring; the line's
\* end (effects.SweepSession) and the trap (night.Trap) sweep it. Tests:
\* TestAR17C_1_SignalMidPlan, TestAR17C_1_SignalMidGate,
\* TestAR17C_2_SetsidDaemonSwept. A SIGKILLed relay's escapee is outside:
\* groups.json records groups, not sessions.
NoOrphanAfterExit ==
  /\ \A r \in Rows : (written[r].sitting \/ relaySt = "reaped") =>
       ~\E p \in Pids : procs[p].alive /\ procs[p].row = r
                        /\ (procs[p].group = row[r].group \/ procs[p].holdsStdout)
  /\ relaySt = "up" => \A e \in Pids : procs[e].alive /\ procs[e].kind = "escaped" =>
                                         measuring # 0 /\ procs[e].parent = measuring

\* Guard: relay.Invariants `unrun-never-passes` at the sitting line and the
\* gate. Test: TestAR10_7_VerifyStatusAndUnrun.
UnrunNeverPasses == \A r \in Rows : row[r].st = "green" => checks[r].gate \in {"green", "same-as-main"}

\* Guard: boundShell's D4 rule (night.LoadedReason: a red read above 2x is
\* unrun: loaded, never stored). Test: TestAR10_5_RedBaselineLoadedUnrun.
NoSameAsMainAgainstLoaded == \A r \in Rows : checks[r].gate = "same-as-main" => ~anchorLoaded[r]

\* Guard: BuildPlan's scope switch (landed copies its last plan line). Test:
\* TestAR10_2_LandedFromLastPlanLine.
LandedNeverMeasured ==
  \A r \in Rows : row[r].st = "landed" =>
    /\ checks[r].base = "none"
    /\ ~\E p \in Alive("measure") : procs[p].row = r

\* Guard: night.landed (satisfied, DepSatisfied) reads Git.FirstParentMerged
\* and the stage's cut, never bare ancestry (AR13 act 8, ar10's gap 1). Tests:
\* TestAR13_8_FreshBranchNotLanded, TestAR13_8_FirstParentMergeLands.
BranchTipMovedBeforeLanded == \A r \in Rows : row[r].st = "landed" => tip[r] # cut[r] /\ r \in mainMerges

\* Guard: gate.gather binds relay.Gate.Head (the row's branch checked out, its
\* tip) and gate.MergeIfGreen merges that sha, refusing a moved tip (ADV6-3),
\* and only on a green verify: a red main never merges (ADV6-5). Tests:
\* TestAR13_0_GateBindsBranchSha, TestAR13_1_RedMainNeverMerges.
MergedShaIsGated ==
  \A r \in Rows : checks[r].merged # 0 =>
    /\ checks[r].merged = checks[r].at
    /\ checks[r].gate = "green"

\* Guard: night.Stage records Staged.BaseSha; gate.gather reds a base not at
\* it nor at the relay's own last merge (main-moved) and gate.MergeIfGreen
\* lands only onto the main the gate read (ADV10-1). Test:
\* TestAR17A_0_GateBindsBase.
NoMergeAfterMainMoved ==
  \A r \in Rows : checks[r].merged # 0 =>
    /\ checks[r].onto = checks[r].main

\* Guard: runVerb's pre-check, night.ErrNested, the hook's nested-run (C4,
\* ADV6-1). Tests: TestAR11_4_NestedRunRefuses, TestAR11_4_NightRunNestedRefuses,
\* TestAR11_4_HookDeniesNestedRun; for a detached one, PidGuard (AR21 act 2):
\* TestAR21_2_DetachedNestedRelay, TestAR21_2_SecondNightRefused.
NoNestedSpawn == \A p \in Alive("child") : procs[p].parent = Relay

\* One relay holds the account lock (AR22 act 2, ADV13-4): the port lock
\* survives Unlink; TestAR22_2_Takeover, TestAR22_2_HolderDeath.
OneHolder == Cardinality(Holders) <= 1


---------------------------------------------------------------------------
(* Liveness: the four properties (§16). The relay's loop, the watchdog and *)
(* the breaker's polls are weakly fair, and a dead relay's next start (the *)
(* reap); the child, the owner, the signal and the load are not. LoadFair  *)
(* adds that a load the breaker waits on falls.                            *)

FairSpec == /\ Spec
            /\ \A r \in Rows : WF_vars(Poll(r)) /\ WF_vars(Stall(r))
            /\ WF_vars(BreakerPoll)
            /\ WF_vars(NightStep)
            /\ WF_vars(Reap)

LoadFair == FairSpec /\ WF_vars(LoadDown)

\* Guard: night.Watchdog in RunRow. Test: TestAR10_6_StallKillsGroup.
StalledChildEventuallyKilled ==
  \A r \in Rows : (row[r].st = "running" /\ stall[r] = StallAt /\ ChildOf(r) # {}) ~> (ChildOf(r) = {})

\* Guard: night.Breaker's Close. Test: TestAR10_5_BreakerOpensAndCloses.
\* Needs LoadFair: under FairSpec alone a load that never falls trips it.
OpenBreakerEventuallyCloses == [](breaker = "open" => <>(breaker = "closed"))

\* Guard: night.Breaker's Max. Test: TestAR10_5_StillOpenSpawnStopsLoad.
OpenBreakerEventuallyResolves == [](breaker = "open" => <>(breaker \in {"closed", "tripped"}))

\* Guards: the wall (night.DefaultWall), the watchdog, the breaker's Max and
\* the halt. Tests: TestAR10_6_WallNeverZero, TestEveryStopHasReason. A
\* killed relay's rows are the next night's to plan: reaped parks them.
EveryRunnableRowEventuallyGatedOrParked ==
  \A r \in Rows : (row[r].st = "ready") ~> (row[r].st \in Terminal \/ relaySt = "reaped")

\* Guard: the collect step's account stop in night.Run (run 10 ruling 1).
\* Tests: TestAR17D_0_UsageLimitStopsNight, TestAR17D_0_ParallelRowsKilled.
\* Once a night stops for the account, no row runs: none is spawned and
\* none still holds a child.
NoSpawnAfterAccountStop ==
  night.reason \in AccountStops => \A r \in Rows : row[r].st \notin {"running", "exited", "swept"}

Bounded == \A p \in Pids : procs[p].depth <= MaxDepth

=============================================================================
