#!/usr/bin/env bash

set -xeuo pipefail

dir="$(dirname $(dirname $(dirname $(dirname "${0}"))))"

if [ -z "${TAGS-}" ]
then
    TAGS=bazel,gss
else
    TAGS="bazel,gss,$TAGS"
fi

bazel build //pkg/cmd/bazci --config=ci
BAZEL_BIN=$(bazel info bazel-bin --config=ci)
ARTIFACTS_DIR=/artifacts

# Query to list all affected tests.
PKG=${PKG#"./"}
if [[ $(basename $PKG) != ... ]]
then
    PKG="$PKG:all"
fi
tests=$(bazel query "kind(go_test, $PKG)" --output=label)

# Run affected tests.
for test in $tests
do
    if [[ ! -z $(bazel query "attr(tags, \"broken_in_bazel\", $test)") ]]
    then
        echo "Skipping test $test as it is broken in bazel"
        continue
    fi
    if [[ ! -z $(bazel query "attr(tags, \"integration\", $test)") ]]
    then
        echo "Skipping test $test as it is an integration test"
        continue
    fi
    exit_status=0
    $BAZEL_BIN/pkg/cmd/bazci/bazci_/bazci -- test --config=ci "$test" \
                                          --test_env=COCKROACH_NIGHTLY_STRESS=true \
                                          --test_timeout="$TESTTIMEOUTSECS" \
                                          --run_under "@com_github_cockroachdb_stress//:stress -bazel -shardable-artifacts 'XML_OUTPUT_FILE=$BAZEL_BIN/pkg/cmd/bazci/bazci_/bazci merge-test-xmls' $STRESSFLAGS" \
                                          --define "gotags=$TAGS" \
                                          --nocache_test_results \
                                          --test_output streamed \
                                          ${EXTRA_BAZEL_FLAGS} \
        || exit_status=$?
    if [ $exit_status -ne 0 ]
    then
        exit $exit_status
    fi
done

