#!/bin/sh
# make tla (design/relay-r1.md §16): TLC over the night; never fetches.
set -u
case ${TLA_JAR:-} in # make's TLA_JAR is the caller's, relative to where make runs
"") jar=.tools/tla2tools.jar ;;
/*) jar=$TLA_JAR ;;
*) jar=$PWD/$TLA_JAR ;;
esac
cd "$(dirname "$0")" || exit 2
JAVA=${JAVA:-java}
[ -f "$jar" ] || { echo "tla: no $jar, run make tla-tools"; exit 2; }
mkdir -p .tools || exit 2
for n in NoNestedMeasure OneMeasurePerRoot NoSpawnWhenOpen NoOrphanAfterExit UnrunNeverPasses \
	NoSameAsMainAgainstLoaded LandedNeverMeasured BranchTipMovedBeforeLanded NoNestedSpawn OneHolder MergedShaIsGated \
	StalledChildEventuallyKilled OpenBreakerEventuallyCloses OpenBreakerEventuallyResolves \
	EveryRunnableRowEventuallyGatedOrParked; do
	grep -q "^$n ==" Night.tla || { echo "tla: Night.tla does not define $n"; exit 2; }
done

tlc() { # config -> TLC's output on stdout
	"$JAVA" -XX:+UseParallelGC -cp "$jar" tlc2.TLC -workers 4 \
		-metadir .tools/states -config "$1" Night.tla 2>&1
}
stats() { # TLC's output -> "N distinct states, Ts"
	d=$(printf '%s\n' "$1" | sed -n 's/.* \([0-9,]*\) distinct states found.*/\1/p' | tail -1)
	t=$(printf '%s\n' "$1" | sed -n 's/^Finished in \(.*\) at .*/\1/p')
	echo "$d distinct states, $t"
}

out=$(tlc Night.cfg)
printf '%s\n' "$out" | grep -q '^Model checking completed. No error has been found.$' || {
	printf '%s\n' "$out" | tail -25; echo "tla: Night.cfg is not clean"; exit 2; }
echo "tla: Night.cfg clean ($(stats "$out"))"
storm() { # config invariant trace: TLC must find the invariant violated
	out=$(tlc "$1")
	printf '%s\n' "$out" | grep -q "^Error: Invariant $2 is violated.$" || {
		printf '%s\n' "$out" | tail -25; echo "tla: $1 did not violate $2"; exit 2; }
	# from TLC's Error line to the last state; its path lines above are a hygiene red
	printf '%s\n' "$out" | awk '/^Error: Invariant/ { on = 1 } on && /states generated/ { exit }
		on && $0 == "" { blank++; next }
		on { while (blank > 0) { print ""; blank-- } print }' > ".tools/$3"
	# four workers pick among the shortest traces, so the committed one is
	# rewritten only when absent or asked (TLA_TRACES=1); its depth must hold
	depth=$(grep -c '^State [0-9]*:' ".tools/$3")
	if [ ! -f "$3" ] || [ "${TLA_TRACES:-}" = 1 ]; then cp ".tools/$3" "$3"; fi
	grep -q "^Error: Invariant $2 is violated.$" "$3" && [ "$(grep -c '^State [0-9]*:' "$3")" = "$depth" ] || {
		echo "tla: $3 is not $1's trace ($depth states deep), run TLA_TRACES=1 make tla"; exit 2; }
	echo "tla: $1 violates $2 ($depth states deep, $3)"
}
storm Storm.cfg NoNestedMeasure storm-trace.txt
storm Storm-run.cfg NoNestedSpawn storm-run-trace.txt
storm Storm-detached.cfg NoNestedSpawn storm-detached-trace.txt
storm Storm-path.cfg OneHolder storm-path-trace.txt
echo "tla: ok"
