"""Compare one node's published supply difference against the baseline.

Reads /api/status JSON on stdin, takes the tolerated difference as argv[1], and
exits non-zero when the ledger and the protocol rule have drifted apart further
than they had.

Kept as a file rather than inline in the workflow so it can be run by hand
against a node, and so the failure messages stay readable.
"""

import json
import re
import sys

# One micro-AEQ of slack. Balances are micro-integers, so a real leak is at
# least this large and anything smaller is not representable.
EPSILON = 1e-6


def main() -> int:
    baseline = float(sys.argv[1])
    try:
        status = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(f"  /api/status did not return JSON: {exc}")
        return 1

    raw = status.get("supply_difference")
    if raw is None:
        print("  /api/status does not publish supply_difference.")
        print("  That field is the only independent signal that the ledger matches")
        print("  the rule. If it was removed, this check is blind — restore it")
        print("  rather than deleting this job.")
        return 1

    match = re.search(r"-?[0-9.]+", str(raw))
    if not match:
        print(f"  supply_difference is {raw!r}, which is not a number")
        return 1
    got = float(match.group(0))

    print(f"  measured difference: {got:.6f} AEQ (baseline {baseline:.6f})")

    if got > baseline + EPSILON:
        print(f"  GREW by {got - baseline:+.6f} AEQ since the baseline.")
        print("  Something is still creating AEQ. The daily-distribution remainder")
        print("  fix went live on 2026-08-19; if this is growing after that, it was")
        print("  not the only path. Do not raise the baseline to make this pass.")
        return 1

    if got < baseline - EPSILON:
        print(f"  SHRANK by {baseline - got:.6f} AEQ.")
        print("  Destroyed supply is the safe direction, but it is unexplained too.")
        print("  The migration write-ordering fix destroys AEQ when a credit fails")
        print("  to persist — check the node logs for [MIGRATE].")
        return 1

    print("  unchanged")
    return 0


if __name__ == "__main__":
    sys.exit(main())
