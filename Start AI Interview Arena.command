#!/bin/bash
# Finder opens this file in Terminal. Keep the complete extracted kit together.
kit_directory="$(cd "$(dirname "$0")" && pwd)"
cd "$kit_directory" || exit 1
if [ ! -x "$kit_directory/ai-interview-arena" ]; then
  printf '%s\n' 'AI Interview Arena: the executable is missing or cannot run.' \
    'Extract the complete ZIP package into a writable folder and try again.' \
    'If you are working from source, run scripts/build-macos.sh.'
  if [ -t 0 ]; then read -r -p 'Press Return to close… ' arena_reply; fi
  exit 1
fi
printf '%s\n' 'AI Interview Arena · local startup' \
  'Keep this window open. Choose Settings > Stop the app in the browser or press Ctrl+C to quit.'
# exec inside the child avoids an extra process between Terminal and the Go program,
# while this small outer shell can keep a startup error visible for Finder users.
( exec "$kit_directory/ai-interview-arena" --root "$kit_directory" "$@" )
arena_status=$?
if [ "$arena_status" -ne 0 ] && [ "$arena_status" -ne 130 ] && [ "$arena_status" -ne 129 ]; then
  printf '\nThe program exited with error code %s. See the message above and README.md.\n' "$arena_status"
  if [ -t 0 ]; then read -r -p 'Press Return to close… ' arena_reply; fi
fi
exit "$arena_status"
