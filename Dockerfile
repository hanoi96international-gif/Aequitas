FROM golang:1.24-alpine AS builder

# cgo, deliberately (measured 2026-07-25 on Contabo1 — the real target
# hardware, not a laptop):
#
#   CGO_ENABLED=0   269.3 us per secp256k1 recovery  ->  13.5 cores at 50k TPS
#   CGO_ENABLED=1   106.3 us per secp256k1 recovery  ->   5.3 cores at 50k TPS
#
# go-ethereum picks its signature implementation at BUILD time:
# crypto/signature_cgo.go binds libsecp256k1 (vendored in the module, no
# system package required) when cgo is available, and otherwise falls back to
# the pure-Go btcec path in signature_nocgo.go. This image had no C toolchain,
# so every node was silently on the slow path — confirmed on the running
# container, whose binary reports "Not a valid dynamic program", i.e. fully
# static, i.e. built without cgo.
#
# Signature recovery is the one per-transaction cost that no amount of I/O or
# lock engineering removes: pure CPU, paid once per transaction, before any
# state is touched. On a 6-core node 13.5 cores is impossible and 5.3 is
# merely expensive — so this flag alone decides whether 50k TPS is reachable
# on the modest hardware the decentralisation constraint requires.
#
# gcc + musl-dev are what libsecp256k1 needs to compile; the runtime stage is
# Alpine as well, so the resulting musl-linked binary runs there unchanged.
RUN apk add --no-cache gcc musl-dev
ENV CGO_ENABLED=1

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The commit this image was built from.
#
# Go stamps vcs.revision automatically, but only when the build can see a
# repository — and .dockerignore excludes .git (correctly: it was 1079 MB of
# a 1.22 GB build context). So every node reported git_commit "unknown", and
# the explorer's Network tab could never answer the one question a rollout
# actually asks: is the whole fleet on the same build?
#
# The deploy scripts pass --build-arg GIT_COMMIT=$(git rev-parse --short HEAD)
# from the checkout they just pulled. Unset, it falls back to "unknown", which
# is what a local `docker build` without the arg should say.
ARG GIT_COMMIT=unknown
# FIX (Audit 2026-08-18): --build-arg alone never actually reached this. The
# ARG above and the -ldflags below were both in place, but nothing in this
# repository ever passed the value: the docker build runs inside
# /root/deploy.sh and /root/deploy_safe_c2.sh, which live only on the boxes
# and are not version-controlled here. So git_commit stayed "unknown" on both
# validators and the explorer's Network tab could never answer the one
# question a rollout asks — is the whole fleet on the same build?
#
# .git-commit-stamp closes that without needing either script. The deploy
# workflows (which ARE in this repository) write it into the checkout right
# before invoking the box script, so it arrives as part of the build context.
# An explicit --build-arg still wins if one is ever passed; the file is the
# fallback; "unknown" remains the answer for a bare local `docker build`,
# which is correct.
RUN GIT_COMMIT_EFFECTIVE="${GIT_COMMIT}"; \
    if [ "$GIT_COMMIT_EFFECTIVE" = "unknown" ] && [ -s .git-commit-stamp ]; then \
      GIT_COMMIT_EFFECTIVE="$(tr -d '[:space:]' < .git-commit-stamp)"; \
    fi; \
    echo "building with git commit: ${GIT_COMMIT_EFFECTIVE}"; \
    go build -ldflags "-X github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper.buildGitCommitStamp=${GIT_COMMIT_EFFECTIVE}" -o aequitasd ./cmd/aequitasd/

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/aequitasd .
COPY --from=builder /app/genesis.json .
COPY --from=builder /app/aequitas-dapp.html .
COPY --from=builder /app/downloads ./downloads
EXPOSE 8080
EXPOSE 4001
CMD ["./aequitasd"]
