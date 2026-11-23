# Records assets/demo.gif. Run from the repo root:
#
#   vhs assets/demo.tape
#
# The demo project is generated in a temp dir while recording is hidden,
# so no deliberately outdated manifests live in this repo.

Output assets/demo.gif

Require go

Set Shell "bash"
Set FontSize 15
Set Width 1240
Set Height 630
Set Padding 24
Set Margin 16
Set MarginFill "#0b0e1a"
Set BorderRadius 10
Set WindowBar Colorful
Set TypingSpeed 75ms
Set Theme { "name": "zephy", "background": "#0f1220", "foreground": "#c9d1e8", "cursor": "#7c5cff", "selection": "#2e3654", "black": "#1a1f36", "brightBlack": "#5a6178", "red": "#ff5d5d", "brightRed": "#ff7b7b", "green": "#3ddc84", "brightGreen": "#5ff0a0", "yellow": "#f7b32b", "brightYellow": "#ffd166", "blue": "#7aa2f7", "brightBlue": "#9fb4ff", "magenta": "#7c5cff", "brightMagenta": "#a48bff", "cyan": "#5fd7d7", "brightCyan": "#87ffff", "white": "#c9d1e8", "brightWhite": "#ffffff" }

Hide
Type@1ms "export PS1='$ ' ZEPHY_OFFLINE=1 && d=$(mktemp -d) && go build -o $d/bin/zephy ./cmd/zephy && export PATH=$d/bin:$PATH && mkdir -p $d/shop-api/.github/workflows && cd $d/shop-api"
Enter
Sleep 8s
Type@1ms "printf 'FROM python:3.9-slim\nRUN pip install -r requirements.txt\n' > Dockerfile"
Enter
Type@1ms "printf 'services:\n  db:\n    image: postgres:12\n  queue:\n    image: rabbitmq:3-management\n  cache:\n    image: redis:7.2\n' > docker-compose.yml"
Enter
Type@1ms "printf 'django==4.2.11\ncelery==5.3.6\n' > requirements.txt"
Enter
Type@1ms `printf '{\n  "name": "web",\n  "engines": {"node": "18.x"},\n  "dependencies": {\n    "request": "^2.88.2",\n    "react": "^18.2.0"\n  },\n  "devDependencies": {"node-sass": "^9.0.0"}\n}\n' > package.json`
Enter
Type@1ms "printf '22\n' > .nvmrc"
Enter
Type@1ms `printf 'jobs:\n  test:\n    runs-on: ubuntu-22.04\n    steps:\n      - uses: actions/setup-python@v5\n        with:\n          python-version: "3.11"\n' > .github/workflows/ci.yml`
Enter
# ttyd answers the TUI's terminal-capability queries (DECRQM) only after
# a fast scan has exited, and the replies land on the prompt. Bubble Tea
# skips those queries over SSH; claiming SSH changes nothing else on screen.
Type@1ms "export SSH_TTY=vhs"
Enter
Type@1ms "clear"
Enter
Sleep 1s
Show

Sleep 800ms
Type "zephy ."
Sleep 700ms
Enter
Sleep 7s
