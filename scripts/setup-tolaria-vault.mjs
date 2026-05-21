import fs from "node:fs";
import path from "node:path";

const root = process.argv[2];

if (!root) {
  throw new Error("Usage: node scripts/setup-tolaria-vault.mjs <vault-path>");
}

if (!fs.existsSync(path.join(root, "AGENTS.md"))) {
  throw new Error(`No AGENTS.md found in ${root}; refusing to scaffold outside a Tolaria vault.`);
}

const dirs = [
  "00-inbox",
  "10-projects",
  "20-areas",
  "30-resources",
  "40-archives",
  "attachments",
  "views",
];

for (const dir of dirs) {
  fs.mkdirSync(path.join(root, dir), { recursive: true });
}

const files = new Map([
  [
    "source.md",
    `---
type: Type
_icon: link
_color: "#2563eb"
_order: 10
_list_properties_display:
  - status
  - para
  - source_url
  - captured_from
---

# Source

A Source is an external item captured for processing: a URL, Slack message, repo, article, video, PDF, or copied note.

status: Inbox
para: "[[Inbox]]"
captured_from:
source_url:
source_title:
source_author:
source_date:
processed_date:
summary:
tags:
`,
  ],
  [
    "project.md",
    `---
type: Type
_icon: target
_color: "#16a34a"
_order: 20
_list_properties_display:
  - status
  - outcome
  - area
---

# Project

A Project is an active outcome with a finish line.

status: Active
outcome:
area:
next_action:
related_sources:
`,
  ],
  [
    "area.md",
    `---
type: Type
_icon: layers
_color: "#9333ea"
_order: 30
_list_properties_display:
  - status
  - standard
---

# Area

An Area is an ongoing responsibility or domain to maintain.

status: Active
standard:
related_projects:
related_sources:
`,
  ],
  [
    "resource.md",
    `---
type: Type
_icon: library
_color: "#ea580c"
_order: 40
_list_properties_display:
  - status
  - topic
---

# Resource

A Resource is reusable reference material organized by topic.

status: Active
topic:
related_sources:
`,
  ],
  [
    "inbox.md",
    `---
type: Area
status: Active
standard: Sources wait here until they are processed into Projects, Areas, Resources, or Archives.
---

# Inbox

This is the holding area for unprocessed captures.
`,
  ],
  [
    "pkm-tolaria.md",
    `---
type: Area
status: Active
standard: Keep this vault useful as a trusted PARA knowledge system maintained by Codex and browsed in Tolaria.
related_sources:
---

# PKM - Tolaria

This area owns the operating model for this vault.

## Working Agreement

Randy provides sources. Codex processes, classifies, summarizes, links, and maintains the vault. Tolaria is Randy's reading, browsing, and editing interface.

## Current Rule

Keep capture simple. New material starts as a Source in Inbox unless it clearly belongs somewhere else.
`,
  ],
  [
    "views/inbox-sources.yml",
    `name: Inbox Sources
icon: inbox
color: "#2563eb"
sort: "modified:desc"
filters:
  all:
    - field: type
      op: equals
      value: Source
    - field: status
      op: equals
      value: Inbox
`,
  ],
  [
    "views/active-projects.yml",
    `name: Active Projects
icon: target
color: "#16a34a"
sort: "modified:desc"
filters:
  all:
    - field: type
      op: equals
      value: Project
    - field: status
      op: equals
      value: Active
`,
  ],
  [
    "views/resources.yml",
    `name: Resources
icon: library
color: "#ea580c"
sort: "title:asc"
filters:
  all:
    - field: type
      op: equals
      value: Resource
`,
  ],
  [
    "views/areas.yml",
    `name: Areas
icon: layers
color: "#9333ea"
sort: "title:asc"
filters:
  all:
    - field: type
      op: equals
      value: Area
`,
  ],
]);

for (const [relative, content] of files) {
  const target = path.join(root, relative);
  if (fs.existsSync(target)) {
    console.log(`exists ${relative}`);
    continue;
  }
  fs.writeFileSync(target, content, "utf8");
  console.log(`created ${relative}`);
}

const agentsPath = path.join(root, "AGENTS.md");
const marker = "## Randy PKM operating model";
let agents = fs.readFileSync(agentsPath, "utf8");

if (!agents.includes(marker)) {
  agents += `

${marker}

- Treat this vault as Randy's source-of-truth knowledge base.
- Tolaria is the user interface; Codex is the maintainer/editor.
- Slack self-DM and user-provided links are capture sources, not long-term storage.
- New captures should become Source notes with source_url, captured_from, summary, status, and para fields.
- Default uncertain captures to status: Inbox and para: "[[Inbox]]".
- Promote material into Project, Area, Resource, or Archive notes only when the destination is clear.
- Preserve original URLs and Slack permalinks whenever available.
- Prefer small, linked notes over long dumping-ground documents.
`;
  fs.writeFileSync(agentsPath, agents, "utf8");
  console.log("updated AGENTS.md");
}

console.log(`Scaffold complete: ${root}`);
