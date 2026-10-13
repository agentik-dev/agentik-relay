---
description: The morning read: the night's report as markdown, or as a narrative through respeak when it is installed
allowed-tools: Bash(relay:*)
argument-hint: "[--night D]"
---

1. Run `relay report $ARGUMENTS` (markdown) and `relay report --json $ARGUMENTS`. If the exit
   is 3 or `relay` is not found, say `relay is not on PATH: make install` and stop.
2. When the `respeak:respeak` agent is available in this session, hand it the markdown in
   `technical` mode and print its narrative. Otherwise print the markdown verbatim.
   respeak is optional: never ask to install it.
3. Whatever the rendering, end with every `resume[]` entry of the JSON verbatim, one per line,
   under `Resume (in your own terminal):`. Each is `relay resume <row>` for a red or failed
   row; never run them here. When `resume` is empty, say `nothing to resume`.
