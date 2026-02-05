# BOSSMAN

task management for clankers 🤖 and the mandem who supervise them innit

## what is this

three ways to chat to a sqlite database yeah:

| mode | who runs it |
|------|-------------|
| `bossman add "ting"` | you boss |
| `bossman mcp` | your clanker |
| `bossman serve` | you (if you're feeling fancy) |

```
MANDEM       CLANKER        DASHBOARD
   |            |               |
   +------------+---------------+
                |
                v
          ~/.bossman/
          bossman.db
```

## install

```sh
go install github.com/procdexeh/bossman@latest
```

## usage

```sh
# get it out your head brev
bossman add "fix the ting"

# see the damage
bossman list

# job done
bossman done task_abc123
```

## for the clankers

drop this in your claude/cursor config:

```json
{
  "mcpServers": {
    "bossman": {
      "command": "bossman",
      "args": ["mcp"]
    }
  }
}
```

now your clanker manages tasks. big man ting.

## philosophy

- sqlite is enough
- one binary is enough  
- you are enough

## status

works on my machine. no refunds brev.

---

