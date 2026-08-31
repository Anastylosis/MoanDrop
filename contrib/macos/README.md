# macOS: right-click "Find subtitles"

macOS support is best-effort — nobody on this project runs a Mac day to
day, so this is untested and there's no installer. If it breaks for you,
patches welcome.

Automator can wrap the CLI into a right-click Quick Action:

1. Open **Automator** → **New Document** → **Quick Action**.
2. Set **Workflow receives current** to `video files` in `Finder`.
3. Add a **Run Shell Script** action, set **Pass input** to
   `as arguments`, and use:

   ```sh
   for f in "$@"; do
       /usr/local/bin/moandrop match "$f" --lang en --write
   done
   ```

   Adjust the `moandrop` path to wherever you installed it (`which
   moandrop` in a terminal), and `--lang en` to your preferred language.
4. Save the Quick Action as "Find subtitles (MoanDrop)".

It now appears under **Finder → right-click a video → Quick Actions**.
Output only goes to Automator's log (Console.app, or run the action from
the Automator editor with a selected file to see it directly) — there's
no notification center integration here, unlike the Linux script's
`notify-send`.
