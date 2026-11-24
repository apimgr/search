# Client GUI and TUI

The client command inventory is the same in the GUI and TUI. Each listed
server-facing CLI leaf has a corresponding screen and view, and actions are
executed through the existing CLI command implementation so authentication,
validation, output formatting, and server behavior remain consistent.

The inventory follows the root help order:

| CLI leaf | GUI screen | TUI view |
| --- | --- | --- |
| `alerts create` | Create alert | Create alert |
| `alerts delete` | Delete alert | Delete alert |
| `alerts get` | Read alert | Read alert |
| `alerts update` | Update alert | Update alert |
| `answer` | Direct answer | Direct answer |
| `autocomplete` | Autocomplete | Autocomplete |
| `autodiscover` | Autodiscover | Autodiscover |
| `bangs` | Bangs | Bangs |
| `categories` | Categories | Categories |
| `engines` | Engines | Engines |
| `health` | Health | Health |
| `info` | Server information | Server information |
| `instant` | Instant answer | Instant answer |
| `metrics` | Metrics | Metrics |
| `preferences` | Preferences | Preferences |
| `preferences set` | Update preferences | Update preferences |
| `related` | Related searches | Related searches |
| `scheduler history` | Scheduler history | Scheduler history |
| `scheduler list` | Scheduler task list | Scheduler task list |
| `scheduler run` | Run scheduler task | Run scheduler task |
| `scheduler show` | Scheduler task details | Scheduler task details |
| `search` | Search | Search |
| `server-config` | Server configuration | Server configuration |
| `widget` | Widget data | Widget data |
| `widgets` | Widgets | Widgets |

In a GUI or TUI command view, enter the same positional arguments and
`key=value` fields accepted by the matching `search-cli` command. Arguments
are passed as separate process arguments; shell expansion is not performed.
The interface shows command output and API error messages in the selected
view. Machine-only endpoints and server maintenance actions are intentionally
not part of the client inventory.
