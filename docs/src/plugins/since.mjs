// Marks the rows of the docs' field and flag tables with the release that
// made the field or flag available, read from the code's history, so nobody
// writes or updates a version by hand.
//
// A field is available from the first release tag whose manifest types carry
// its yaml tag (internal/manifest), a flag from the first tag whose commands
// define it (cmd). Tags are walked oldest first, and the working tree last:
// a name found only there is not released yet and is marked Unreleased.
//
// Only what came after BASELINE is marked; older fields would all carry a
// badge and say nothing. Names are matched without their parent, so a new
// field named like an older one elsewhere (name, enabled) is not marked.
//
// The docs are published with a release, from the commit released, so the
// release's own tag already exists when they are built. Without tags (a
// shallow clone) nothing is marked.
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

export const BASELINE = 'v0.15.0';

const repoRoot = fileURLToPath(new URL('../../../', import.meta.url));

const git = (...args) =>
  execFileSync('git', args, { cwd: repoRoot, encoding: 'utf8', maxBuffer: 64 << 20 });

// grep is the POSIX ERE git grep runs; re picks the name out of each match.
export const FIELD = {
  pathspec: ['internal/manifest/*.go', ':!*_test.go'],
  grep: 'yaml:"[a-z0-9_]+',
  re: /yaml:"([a-z0-9_]+)/g,
  key: (m) => m,
};
export const FLAG = {
  pathspec: ['cmd/*.go', ':!*_test.go'],
  grep: 'Flags\\(\\)\\.[A-Za-z]+\\([^"]*"[a-z0-9-]+"',
  re: /Flags\(\)\.[A-Za-z]+\([^"]*"([a-z0-9-]+)"/g,
  key: (m) => `--${m}`,
};

/** The names kind defines in source (Go code, or git grep's matches of it). */
export function namesIn(source, kind) {
  return new Set([...source.matchAll(kind.re)].map((m) => kind.key(m[1])));
}

/** Names found by kind in the files of rev (a tag, or null for the working tree). */
function namesAt(rev, kind) {
  let out = '';
  try {
    out = git('grep', '-h', '-o', '-E', kind.grep, ...(rev ? [rev] : []), '--', ...kind.pathspec);
  } catch {
    // git grep exits 1 when nothing matches
  }
  return namesIn(out, kind);
}

export function cmpVersion(a, b) {
  const pa = a.replace(/^v/, '').split(/[.-]/).map(Number);
  const pb = b.replace(/^v/, '').split(/[.-]/).map(Number);
  for (let i = 0; i < 3; i++) if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) - (pb[i] || 0);
  return 0;
}

/**
 * Name to the release it arrived in, after BASELINE only. snapshots are
 * [version, names] oldest first, the working tree last as [null, names]; a
 * name only there is 'Unreleased'.
 */
export function firstReleases(snapshots, baseline = BASELINE) {
  const first = new Map();
  for (const [version, names] of snapshots) {
    for (const name of names) if (!first.has(name)) first.set(name, version ?? 'Unreleased');
  }
  const since = new Map();
  for (const [name, version] of first) {
    if (version === 'Unreleased' || cmpVersion(version, baseline) > 0) since.set(name, version);
  }
  return since;
}

/** firstReleases over this repository's release tags and working tree. */
export function buildSince() {
  let tags = [];
  try {
    tags = git('tag', '--list', 'v*', '--sort=v:refname').split('\n').filter((t) => /^v\d+\.\d+\.\d+$/.test(t));
  } catch {
    return new Map();
  }
  if (!tags.length) {
    console.warn('[since] no release tags; the docs are built without version badges');
    return new Map();
  }
  return firstReleases(
    [...tags, null].map((rev) => [rev, new Set([...namesAt(rev, FIELD), ...namesAt(rev, FLAG)])]),
  );
}

/** `-o, --output <format>` -> --output; `contexts[].app` -> app */
export function nameOf(code, isFlag) {
  if (isFlag) return code.match(/--[a-z0-9-]+/)?.[0];
  const last = code.trim().split(/\s/)[0].split('.').pop();
  return last?.replace(/\[\]$/, '');
}

const text = (node) =>
  node.type === 'text' || node.type === 'inlineCode' ? node.value : (node.children ?? []).map(text).join('');

/**
 * A Sätteri mdast plugin (Astro's Markdown processor) marking field and flag
 * tables, from since (name to release; this repository's by default).
 */
export default function sincePlugin(since = buildSince()) {
  if (!since.size) return null;
  return {
    name: 'gi-since',
    table(table, ctx) {
      const [head, ...rows] = table.children;
      const header = text(head?.children[0] ?? {}).trim();
      if (header !== 'Field' && header !== 'Flag') return;
      for (const row of rows) {
        const cell = row.children[0];
        const code = cell?.children.find((c) => c.type === 'inlineCode');
        if (!code) continue;
        const name = nameOf(code.value, header === 'Flag');
        const version = name && since.get(name);
        if (!version) continue;
        const unreleased = version === 'Unreleased';
        const title = unreleased ? 'Not in a release yet' : `Available since ${version}`;
        const badge = unreleased ? 'Unreleased' : `${version}+`;
        ctx.appendChild(cell, {
          type: 'html',
          value: `<span class="gi-since${unreleased ? ' is-unreleased' : ''}" title="${title}">${badge}</span>`,
        });
      }
    },
  };
}
