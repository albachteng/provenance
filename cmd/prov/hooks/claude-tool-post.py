#!/usr/bin/env python3
import json
import sys
import subprocess

hook_input = json.load(sys.stdin)

event = {
    'hook_event_name': 'PostToolUse',
    'session_id': hook_input.get('session_id'),
    'tool_name': hook_input.get('tool_name'),
    'tool_use_id': hook_input.get('tool_use_id'),
    'tool_input': hook_input.get('tool_input'),
    'tool_response': hook_input.get('tool_response'),
    'cwd': hook_input.get('cwd'),
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
