# Assigns each package on stdin to the least loaded of n shards, heaviest first, and prints shard i's.
# -v table=<package<TAB>seconds file> weighs packages; one missing from it weighs 5 s.
BEGIN {
  while ((getline line < table) > 0) {
    split(line, f, "\t")
    secs[f[1]] = f[2] + 0
  }
}
{ pkg[NR] = $0; w[NR] = ($0 in secs) ? secs[$0] : 5 }
END {
  for (k = 1; k <= NR; k++) {
    j = k - 1
    while (j >= 1 && (w[order[j]] < w[k] || (w[order[j]] == w[k] && pkg[order[j]] > pkg[k]))) {
      order[j + 1] = order[j]
      j--
    }
    order[j + 1] = k
  }
  for (s = 1; s <= n; s++) load[s] = 0
  for (k = 1; k <= NR; k++) {
    idx = order[k]
    best = 1
    for (s = 2; s <= n; s++) if (load[s] < load[best]) best = s
    load[best] += w[idx]
    shard[idx] = best
  }
  for (k = 1; k <= NR; k++) if (shard[k] == i) print pkg[k]
}
