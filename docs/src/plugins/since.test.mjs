import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { describe, it } from 'node:test';
import { markdownToHtml } from 'satteri';
import sincePlugin, { buildSince, cmpVersion, FIELD, FLAG, firstReleases, nameOf, namesIn } from './since.mjs';

describe('namesIn', () => {
  it('reads yaml tags as fields', () => {
    const go = `
type Security struct {
	SecretScanning *bool \`yaml:"secret_scanning,omitempty"\`
	Name           string \`yaml:"name" validate:"required"\`
	Inline         Spec   \`yaml:",inline"\`
	Skipped        string \`yaml:"-"\`
}`;
    assert.deepEqual(namesIn(go, FIELD), new Set(['secret_scanning', 'name']));
  });

  it('reads flag definitions as flags', () => {
    const go = `
	cmd.Flags().StringVarP(&repo, "repo", "r", "", "target repository")
	cmd.Flags().BoolVar(&detailedExit, "detailed-exitcode", false, "exit 2 on changes")
	cmd.PersistentFlags().IntVar(&n, "concurrency", 8, "gh processes at once")
	cmd.Flags().StringSliceVar(&files, "changed-since", nil, "only changed")`;
    assert.deepEqual(namesIn(go, FLAG), new Set(['--repo', '--detailed-exitcode', '--concurrency', '--changed-since']));
  });

  it('finds what git grep is asked for', () => {
    // git grep runs kind.grep (POSIX ERE); a match it returns has to be one re reads
    for (const [kind, line] of [
      [FIELD, 'X *bool `yaml:"secret_scanning,omitempty"`'],
      [FLAG, 'cmd.Flags().BoolVar(&ci, "ci", false, "deprecated")'],
    ]) {
      const match = line.match(new RegExp(kind.grep))?.[0];
      assert.ok(match, `${kind.grep} matches ${line}`);
      assert.equal(namesIn(match, kind).size, 1);
    }
  });
});

describe('cmpVersion', () => {
  it('compares numerically', () => {
    assert.ok(cmpVersion('v0.9.0', 'v0.10.0') < 0);
    assert.ok(cmpVersion('v0.15.1', 'v0.15.0') > 0);
    assert.equal(cmpVersion('v1.2.3', 'v1.2.3'), 0);
  });
});

describe('firstReleases', () => {
  const snapshots = [
    ['v0.15.0', new Set(['name', 'labels'])],
    ['v0.15.1', new Set(['name', 'labels', 'reconcile'])],
    ['v0.18.0', new Set(['name', 'reconcile', 'environments'])],
    [null, new Set(['name', 'reconcile', 'environments', 'pages'])],
  ];
  const since = firstReleases(snapshots, 'v0.15.0');

  it('keeps the first release a name appears in', () => {
    assert.equal(since.get('reconcile'), 'v0.15.1');
    assert.equal(since.get('environments'), 'v0.18.0');
  });

  it('leaves out what the baseline release already had', () => {
    assert.equal(since.has('name'), false);
    assert.equal(since.has('labels'), false);
  });

  it('marks a name only in the working tree as unreleased', () => {
    assert.equal(since.get('pages'), 'Unreleased');
  });
});

describe('nameOf', () => {
  it('reads the field out of a row', () => {
    assert.equal(nameOf('secret_scanning', false), 'secret_scanning');
    assert.equal(nameOf('reconcile.deployment_branch_policies', false), 'deployment_branch_policies');
    assert.equal(nameOf('contexts[].app', false), 'app');
    assert.equal(nameOf('custom_branch_policies: [...]', false), 'custom_branch_policies:');
  });

  it('reads the long flag out of a row', () => {
    assert.equal(nameOf('--changed-since <ref>', true), '--changed-since');
    assert.equal(nameOf('-o, --output <format>', true), '--output');
    assert.equal(nameOf('-r', true), undefined);
  });
});

describe('sincePlugin', () => {
  const since = new Map([
    ['secret_scanning', 'v0.21.0'],
    ['pages', 'Unreleased'],
    ['--concurrency', 'v0.17.0'],
  ]);
  const render = (md) => markdownToHtml(md, { mdastPlugins: [sincePlugin(since)] }).html;

  it('marks the rows of a Field table', () => {
    const html = render(
      '| Field | Type |\n|---|---|\n| `secret_scanning` | bool |\n| `vulnerability_alerts` | bool |\n| `pages` | object |\n',
    );
    assert.match(html, /<code>secret_scanning<\/code><span class="gi-since" title="Available since v0\.21\.0">v0\.21\.0\+<\/span>/);
    assert.match(html, /<code>vulnerability_alerts<\/code><\/td>/);
    assert.match(html, /<code>pages<\/code><span class="gi-since is-unreleased" title="Not in a release yet">Unreleased<\/span>/);
  });

  it('marks the rows of a Flag table', () => {
    const html = render('| Flag | Description |\n|---|---|\n| `--concurrency <n>` | gh processes at once |\n');
    assert.match(html, /<code>--concurrency &lt;n&gt;<\/code><span class="gi-since"[^>]*>v0\.17\.0\+<\/span>/);
  });

  it('leaves other tables alone', () => {
    const html = render('| Value | Description |\n|---|---|\n| `secret_scanning` | a value |\n');
    assert.doesNotMatch(html, /gi-since/);
  });

  it('leaves a row whose first cell has no code alone', () => {
    const html = render('| Field | Type |\n|---|---|\n| secret_scanning | bool |\n');
    assert.doesNotMatch(html, /gi-since/);
  });

  it('is left out with nothing to mark', () => {
    assert.equal(sincePlugin(new Map()), null);
  });
});

// The releases are history, so these stay true; they fail when reading the
// repository's tags or code breaks. Skipped without tags (a shallow clone).
const hasTags = (() => {
  try {
    return execFileSync('git', ['tag', '--list', 'v0.21.0'], { encoding: 'utf8' }).trim() !== '';
  } catch {
    return false;
  }
})();

describe('buildSince on this repository', { skip: !hasTags && 'no release tags' }, () => {
  const since = buildSince();

  it('finds the release of fields and flags from the tags', () => {
    assert.equal(since.get('secret_scanning'), 'v0.21.0');
    assert.equal(since.get('environments'), 'v0.18.0');
    assert.equal(since.get('build_type'), 'v0.20.0');
    assert.equal(since.get('--changed-since'), 'v0.16.0');
    assert.equal(since.get('--concurrency'), 'v0.17.0');
  });

  it('leaves out what v0.15.0 already had', () => {
    assert.equal(since.has('name'), false);
    assert.equal(since.has('--output'), false);
  });
});
