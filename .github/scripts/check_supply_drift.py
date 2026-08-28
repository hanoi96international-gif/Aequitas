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
        print("  Destroyed supply is the safe direction, but it is still a loss.")
        print()
        print("  KNOWN CAUSE as of 2026-08-28, for a shrink of about 0.000014 AEQ:")
        print("  every distribution floors each share to six decimals and then zeroes")
        print("  the whole pool (state.go, three sites: UBI, validators, LP). What lies")
        print("  between 'n floored shares' and 'the full pool' is destroyed each round.")
        print()
        print("  floor6 is correct and must stay: round6 used to MINT, because the")
        print("  rounded shares summed to more than the pool being zeroed. The error is")
        print("  the zeroing, not the flooring. The fix is to subtract what was actually")
        print("  paid out, so the remainder carries into the next round.")
        print()
        print("  That is a CONSENSUS change -- the pool balance is in the state root, so")
        print("  every node has to switch at the same block, not at the moment it is")
        print("  deployed. It needs an activation height threaded into the distribution")
        print("  path, which is not where you want to be careless. Deliberately not")
        print("  rushed in on 2026-08-28.")
        print()
        print("  A LARGER shrink, or one that keeps growing, is NOT this: check the node")
        print("  logs for [MIGRATE] -- the migration write-ordering fix destroys AEQ")
        print("  when a credit fails to persist.")
        return 1

    print("  unchanged")
    return 0


if __name__ == "__main__":
    sys.exit(main())
