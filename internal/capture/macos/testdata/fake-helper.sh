#!/bin/sh

if [ "$1" = "list" ]; then
  printf '%s\n' '{"type":"target","bundle_id":"example.app","application_name":"Example","pid":1234,"windows":[{"window_id":7,"title":"Example window","on_screen":true,"active":true,"x":0,"y":0,"width":800,"height":600}]}'
  exit 0
fi

printf '%s\n' 'this is not JSON'
printf '%s\n' '{"type":"ready","bundle_id":"example.app","session_id":"fixture"}'
