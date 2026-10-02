"""Exercise dependency graph generation without downloading external tools."""

import os
import pathlib
import subprocess
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).with_name("generate-dep-graph.sh")


class DependencyGraphTests(unittest.TestCase):
    def test_install_with_project_go_version(self):
        self.generate_graph(install=True)

    def test_reuse_explicit_binary(self):
        self.generate_graph(install=False)

    def generate_graph(self, *, install):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            tools = root / "tools"
            tools.mkdir()
            gopath = root / "gopath"
            (gopath / "bin").mkdir(parents=True)
            goda = root / "goda-fixture"
            goda.write_text(
                "#!/bin/bash\nset -eu\n"
                '[[ "$*" == "graph -short transitive(github.com/kamo-naoyuki/rotari/...)" ]]\n'
                'printf "digraph { a -> b; }\\n"\n'
            )
            goda.chmod(0o755)
            go = tools / "go"
            go.write_text(
                "#!/bin/bash\nset -eu\n"
                'if [[ "$*" == "env GOPATH" ]]; then\n'
                '    printf "%s\\n" "$TEST_GOPATH"\n'
                'elif [[ "$1" == "install" ]]; then\n'
                '    [[ "$GOFLAGS" == "-mod=mod" ]]\n'
                '    if [[ "$2" != "github.com/loov/goda@v0.7.1" ]]; then\n'
                '        echo "goda requires Go newer than the project minimum" >&2\n'
                "        exit 1\n"
                "    fi\n"
                '    cp "$TEST_GODA" "$TEST_GOPATH/bin/goda"\n'
                "else\n"
                "    exit 1\n"
                "fi\n"
            )
            go.chmod(0o755)
            dot = tools / "dot"
            dot.write_text(
                "#!/bin/bash\nset -eu\n"
                '[[ "$1" == "-Tsvg" && "$3" == "-o" ]]\n'
                'grep -q "a -> b" "$2"\n'
                'printf "<svg/>\\n" > "$4"\n'
            )
            dot.chmod(0o755)
            env = {
                **os.environ,
                "PATH": f"{tools}:/usr/bin:/bin",
                "TEST_GOPATH": str(gopath),
                "TEST_GODA": str(goda),
                "GODA_BINARY": str(gopath / "bin" / "goda") if install else str(goda),
            }
            output = root / "output with spaces"
            result = subprocess.run(
                ["bash", str(SCRIPT), str(output)],
                env=env,
                text=True,
                capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual((output / "rotari-deps.svg").read_text(), "<svg/>\n")
            self.assertEqual("installing goda@" in result.stdout, install)


if __name__ == "__main__":
    unittest.main()
