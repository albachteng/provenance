#!/usr/bin/env python3
import json
import sys
import subprocess

hook_input = json.load(sys.stdin)

event = {
    'hook_event_name': 'PreToolUse',
    'session_id': hook_input.get('session_id'),
    'tool_name': hook_input.get('tool_name'),
    'tool_use_id': hook_input.get('tool_use_id'),
    'tool_input': hook_input.get('tool_input'),
    'cwd': hook_input.get('cwd'),
    'permission_mode': hook_input.get('permission_mode'),
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
