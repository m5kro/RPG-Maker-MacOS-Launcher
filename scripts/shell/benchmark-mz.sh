#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
base="$PWD/build/validation/mz-20260717"
game=${MZ_BENCHMARK_GAME:-"$base/game/rpgmaker-mz-demo-81abd876e3c339fbd7d74323ad8817ad7011cade"}
runtime=${MZ_BENCHMARK_RUNTIME:-"$base/runtime/v0.94.1/nwjs.app"}
mkdir -p build/benchmark-mz
output=$(mktemp -d "$PWD/build/benchmark-mz/run.XXXXXXXX")
printf 'Benchmark output: %s/run\n' "$output"
go -C src/go run ./cmd/benchmark-mz --fixed --game "$game" --runtime "$runtime" --root "$output/run"
