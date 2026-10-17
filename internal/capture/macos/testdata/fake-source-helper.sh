#!/bin/sh

spool=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --spool)
      spool="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

mkdir -p "$spool"
chunk="$spool/remote-1-0.wav"
if [ "${FAKE_INVALID_WAV:-0}" = "1" ]; then
  printf '%s' 'not a wav file' > "$chunk"
else
  printf '%s' 'UklGRigAAABXQVZFZm10IBAAAAABAAEAgD4AAAB9AAACABAAZGF0YQQAAAABAP//' |
    base64 -D > "$chunk"
fi

printf '%s\n' '{"type":"ready","bundle_id":"example.app","session_id":"fixture"}'
printf '%s\n' "{\"type\":\"audio_chunk\",\"source\":\"remote\",\"path\":\"$chunk\",\"start_ms\":0,\"end_ms\":0,\"sample_rate\":16000,\"channels\":1}"

trap 'exit 0' INT TERM
while :; do
  sleep 1
done
