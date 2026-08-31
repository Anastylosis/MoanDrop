#!/bin/sh
# Nautilus (also Nemo/Caja) script: right-click one or more videos, run
# `moandrop match --write` on each, and report the outcome via notify-send.
#
# Install: copy to ~/.local/share/nautilus/scripts/, chmod +x it. Selection
# reaches scripts two ways depending on file manager/version, so this reads
# argv first and falls back to the env var. Set MOANDROP_LANG to change the
# default target language (Nautilus scripts run with no way to prompt).
set -u

notify() {
    if command -v notify-send >/dev/null 2>&1; then
        notify-send "MoanDrop" "$1"
    else
        printf 'MoanDrop: %s\n' "$1" >&2
    fi
}

lang="${MOANDROP_LANG:-en}"

if [ "$#" -eq 0 ] && [ -n "${NAUTILUS_SCRIPT_SELECTED_FILE_PATHS:-}" ]; then
    # Newline-separated selection; disable globbing so a literal * or ? in a
    # filename doesn't get expanded when we split it into "$@".
    old_ifs=$IFS
    IFS='
'
    set -f
    set -- $NAUTILUS_SCRIPT_SELECTED_FILE_PATHS
    set +f
    IFS=$old_ifs
fi

if [ "$#" -eq 0 ]; then
    notify "no files selected"
    exit 1
fi

for f in "$@"; do
    base=$(basename -- "$f")
    out=$(moandrop match "$f" --lang "$lang" --write 2>&1)
    status=$?
    case "$status" in
    0)
        # writeMatches prints "wrote <path>" or "replaced <path>" per
        # language written; report every sidecar it produced.
        sidecars=$(printf '%s\n' "$out" | sed -n -e 's/^wrote //p' -e 's/^replaced //p')
        msg=""
        while IFS= read -r line; do
            [ -n "$line" ] || continue
            if [ -z "$msg" ]; then
                msg=$line
            else
                msg="$msg; $line"
            fi
        done <<EOF
$sidecars
EOF
        if [ -n "$msg" ]; then
            notify "$base: $msg"
        else
            notify "$base: subtitle written"
        fi
        ;;
    2)
        notify "$base: no match found"
        ;;
    *)
        reason=$(printf '%s\n' "$out" | tail -n 1)
        notify "$base: error${reason:+ - $reason}"
        ;;
    esac
done
