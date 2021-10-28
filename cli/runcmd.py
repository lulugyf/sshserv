import time
import sys

n = int(sys.argv[1]) if len(sys.argv) > 1 else 20
for i in range(n):
    print(f"to stdout {i}", file=sys.stdout)
    print(f"to stderr {i}", file=sys.stderr)
    time.sleep(0.5)
