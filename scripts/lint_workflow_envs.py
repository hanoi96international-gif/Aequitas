#!/usr/bin/env python3
"""Fail if a workflow step forwards a variable over SSH that it never sets.

appleboy/ssh-action has two separate knobs that look like one:

    with:
      envs: VALUE,DRY        # which variables to forward to the REMOTE shell
    env:
      VALUE: ${{ inputs.value }}   # what puts them in the LOCAL step env

`envs:` alone forwards nothing, because there is nothing to forward. The
remote script then runs with the variable unset, which fails in whichever way
that script happens to fail -- and the two blocks sit far enough apart in the
file that the omission reads as complete.

This was written after making the identical mistake twice within ten minutes,
in set-multi-block-tick-contabo2.yml and check-loadtest-account-balances.yml.
Both failed safely (an input guard and `set -u` respectively) but both wasted
a dispatch against a live node, and neither failure pointed at the real cause.

Checked here rather than left to review because it is exactly the kind of
mistake review does not catch: nothing is wrong on either line, only between
them.

A name is considered satisfied if the step's own `env:` defines it, or the job
or workflow does. Anything a workflow deliberately expects from the ambient
runner environment can be listed in ALLOWED_AMBIENT below.
"""

import re
import sys
import pathlib
import yaml

# Variables a step may forward without declaring: set by the runner itself,
# not by us. Keep this list short and justified -- every entry is a hole.
ALLOWED_AMBIENT = {
    "GITHUB_SHA",
    "GITHUB_REF",
    "GITHUB_RUN_ID",
    "GITHUB_REPOSITORY",
    "GITHUB_ACTOR",
    "HOME",
}


def declared_names(*envs):
    out = set()
    for env in envs:
        if isinstance(env, dict):
            out |= set(env.keys())
    return out


# `echo "NAME=value" >> "$GITHUB_ENV"` in an earlier step of the same job.
#
# The runner reads that file between steps and puts every name in it into the
# PROCESS environment of every later step -- which is exactly where
# appleboy/ssh-action's `envs:` looks. So a name set this way is genuinely
# forwarded, and flagging it is a false alarm.
#
# That distinction is worth the code: a linter that is red for no reason gets
# ignored, and then it stops catching the thing it exists for. This one was
# reporting four such names across two workflows -- mpc-client-token.yml
# deliberately routes its token through GITHUB_ENV precisely so the value never
# appears in a `with:` block or a log.
_GITHUB_ENV_WRITE = re.compile(
    r"""(?mx)
    ^\s*echo \s+ ["']?          # echo, optionally quoted argument
    (?P<name>[A-Za-z_][A-Za-z0-9_]*)  # NAME
    = .*? >> \s* ["']? \$\{?GITHUB_ENV
    """
)


def names_set_via_github_env(steps, before_index):
    """Names an earlier step in this job wrote to $GITHUB_ENV."""
    out = set()
    for step in steps[:before_index]:
        if not isinstance(step, dict):
            continue
        run = step.get("run")
        if isinstance(run, str):
            out |= {m.group("name") for m in _GITHUB_ENV_WRITE.finditer(run)}
    return out


def main(paths):
    failures = []
    for path in sorted(paths):
        try:
            # encoding is explicit because read_text() otherwise uses the
            # platform default, which is cp1252 on Windows. Several workflows
            # contain UTF-8 punctuation, so running this linter locally on
            # Windows died with a UnicodeDecodeError before reaching a single
            # check - it only ever worked on the UTF-8 CI runners.
            doc = yaml.safe_load(path.read_text(encoding="utf-8"))
        except (yaml.YAMLError, UnicodeDecodeError) as exc:
            failures.append(f"{path}: not valid YAML: {exc}")
            continue
        if not isinstance(doc, dict):
            continue
        workflow_env = doc.get("env")
        for job_name, job in (doc.get("jobs") or {}).items():
            if not isinstance(job, dict):
                continue
            job_env = job.get("env")
            for idx, step in enumerate(job.get("steps") or []):
                if not isinstance(step, dict):
                    continue
                forwarded = ((step.get("with") or {}).get("envs") or "").strip()
                if not forwarded:
                    continue
                names = [n.strip() for n in forwarded.split(",") if n.strip()]
                have = declared_names(step.get("env"), job_env, workflow_env)
                have |= names_set_via_github_env(job.get("steps") or [], idx)
                missing = [n for n in names if n not in have and n not in ALLOWED_AMBIENT]
                if missing:
                    label = step.get("name") or f"step #{idx + 1}"
                    failures.append(
                        f"{path}: job '{job_name}', {label}: forwards {', '.join(missing)} "
                        f"via `envs:` but no `env:` block defines "
                        f"{'it' if len(missing) == 1 else 'them'}"
                    )

    if failures:
        print("Workflow env forwarding check FAILED:\n")
        for f in failures:
            print(f"  ✗ {f}")
        print(
            "\nAdd an `env:` block to the step (sibling of `uses:`/`with:`) "
            "defining every name listed in `envs:`."
        )
        return 1
    print(f"Workflow env forwarding check passed ({len(paths)} workflow file(s)).")
    return 0


if __name__ == "__main__":
    root = pathlib.Path(__file__).resolve().parent.parent / ".github" / "workflows"
    sys.exit(main(list(root.glob("*.yml")) + list(root.glob("*.yaml"))))
