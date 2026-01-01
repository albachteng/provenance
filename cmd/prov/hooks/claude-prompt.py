#!/usr/bin/env python3
import json
import sys
import subprocess

hook_input = json.load(sys.stdin)

event = {
    'hook_event_name': hook_input.get('hook_event_name'),
    'session_id': hook_input.get('session_id'),
    'prompt': hook_input.get('prompt'),
    'cwd': hook_input.get('cwd'),
    'permission_mode': hook_input.get('permission_mode'),
    'transcript_path': hook_input.get('transcript_path'),
}

try:
    subprocess.run(
        ['{{PROV_PATH}}', 'capture-hook', '--json'],
        input=json.dumps(event),
        text=True,
        check=False,
        capture_output=True
    )
except Exception:
    pass

sys.exit(0)
