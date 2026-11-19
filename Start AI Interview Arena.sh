#!/bin/sh
kit_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || exit 1
cd -- "$kit_directory" || exit 1

if [ ! -x ./ai-interview-arena ]; then
  printf '%s\n' 'AI Interview Arena: the executable is missing or cannot run.' \
    'Extract the complete ZIP package into a writable folder and try again.' >&2
  exit 1
fi

printf '%s\n' 'AI Interview Arena · local startup' \
  'Keep this window open. Choose Settings > Stop the app in the browser or press Ctrl+C to quit.'
exec ./ai-interview-arena --root "$kit_directory" "$@"
