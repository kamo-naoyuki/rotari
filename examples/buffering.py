"""Compare buffered and flushed stdout in a local rotari job.

From the repository root, add either command and run the project:
    rotari add -p buffering -- python3 examples/buffering.py
    rotari add -p buffering -- python3 examples/buffering.py --flush

Use show ATTEMPT_ID while the job runs. With normal Python settings, the
first command buffers stdout when it is redirected to a log; --flush emits
each line immediately. Python -u or PYTHONUNBUFFERED also disables buffering.
"""

import argparse
import sys
import time


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--flush", action="store_true", help="flush each line")
    parser.add_argument("--count", type=int, default=10, help="number of lines")
    parser.add_argument("--interval", type=float, default=2.0, help="seconds per line")
    args = parser.parse_args()
    if args.count < 1 or args.interval < 0:
        parser.error("--count must be positive and --interval must be non-negative")

    print(f"stdout.isatty={sys.stdout.isatty()}, flush={args.flush}", flush=args.flush)
    for index in range(1, args.count + 1):
        print(f"{time.strftime('%H:%M:%S')} tick {index}/{args.count}", flush=args.flush)
        time.sleep(args.interval)
    print("done", flush=args.flush)


if __name__ == "__main__":
    main()
