// Expressive Code (code block) configuration.
// The syntax themes are grayscale on purpose: keys are muted, values are
// bright, and only diff insertions and deletions carry a hue. Frame colors
// come from the --gi-* tokens in src/styles/custom.css.
import { defineEcConfig, ExpressiveCodeTheme } from '@astrojs/starlight/expressive-code';

function neutralTheme({ name, type, bg, fg, key, muted, comment, add, del }) {
  return new ExpressiveCodeTheme({
    name,
    type,
    colors: {
      'editor.background': bg,
      'editor.foreground': fg,
    },
    tokenColors: [
      { settings: { foreground: fg } },
      { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: comment } },
      {
        scope: [
          'entity.name.tag',
          'support.type.property-name',
          'meta.object-literal.key',
          'variable.other.key',
          'variable.parameter',
          'variable.other',
          'constant.other.option',
        ],
        settings: { foreground: key },
      },
      { scope: ['punctuation', 'keyword.operator', 'meta.brace'], settings: { foreground: muted } },
      { scope: ['string', 'constant', 'entity.name.function', 'support.function'], settings: { foreground: fg } },
      { scope: ['keyword', 'storage'], settings: { foreground: key } },
      { scope: ['markup.heading', 'markup.bold'], settings: { foreground: fg, fontStyle: 'bold' } },
      { scope: ['markup.inserted', 'punctuation.definition.inserted'], settings: { foreground: add } },
      { scope: ['markup.deleted', 'punctuation.definition.deleted'], settings: { foreground: del } },
      { scope: ['meta.diff.range', 'meta.diff.header', 'meta.diff.index'], settings: { foreground: comment } },
    ],
  });
}

const dark = neutralTheme({
  name: 'gh-infra-dark',
  type: 'dark',
  bg: '#141414',
  fg: '#ededed',
  key: '#a1a1a1',
  muted: '#8f8f8f',
  comment: '#808080',
  add: '#3fb950',
  del: '#f85149',
});

const light = neutralTheme({
  name: 'gh-infra-light',
  type: 'light',
  bg: '#fafafa',
  fg: '#171717',
  key: '#666666',
  muted: '#7a7a7a',
  comment: '#737373',
  add: '#1a7f37',
  del: '#cf222e',
});

export default defineEcConfig({
  themes: [dark, light],
  useStarlightUiThemeColors: false,
  styleOverrides: {
    borderRadius: '10px',
    borderColor: 'var(--gi-line)',
    borderWidth: '1px',
    codeBackground: 'var(--gi-code)',
    codeFontFamily: 'var(--__sl-font-mono)',
    codeFontSize: '0.8125rem',
    codeLineHeight: '1.7',
    codePaddingBlock: '0.875rem',
    codePaddingInline: '1rem',
    uiFontFamily: 'var(--__sl-font)',
    uiFontSize: '0.75rem',
    scrollbarThumbColor: 'var(--gi-line2)',
    scrollbarThumbHoverColor: 'var(--gi-fg3)',
    frames: {
      frameBoxShadowCssValue: 'none',
      editorTabBarBackground: 'var(--gi-code)',
      editorTabBarBorderColor: 'var(--gi-line)',
      editorTabBarBorderBottomColor: 'var(--gi-line)',
      editorActiveTabBackground: 'var(--gi-code)',
      editorActiveTabForeground: 'var(--gi-fg3)',
      editorActiveTabBorderColor: 'transparent',
      editorActiveTabIndicatorTopColor: 'transparent',
      // Draw the tab bar's bottom line under the title tab too.
      editorActiveTabIndicatorBottomColor: 'var(--gi-line)',
      editorBackground: 'var(--gi-code)',
      terminalBackground: 'var(--gi-code)',
      terminalTitlebarBackground: 'var(--gi-code)',
      terminalTitlebarForeground: 'var(--gi-fg3)',
      terminalTitlebarBorderBottomColor: 'var(--gi-line)',
      terminalTitlebarDotsOpacity: '0',
      inlineButtonBackground: 'var(--gi-fg)',
      inlineButtonForeground: 'var(--gi-fg3)',
      inlineButtonBorder: 'transparent',
      tooltipSuccessBackground: 'var(--gi-fg)',
      tooltipSuccessForeground: 'var(--gi-bg)',
    },
    textMarkers: {
      insBackground: 'color-mix(in srgb, var(--gi-add) 12%, transparent)',
      insBorderColor: 'var(--gi-add)',
      insDiffIndicatorColor: 'var(--gi-add)',
      delBackground: 'color-mix(in srgb, var(--gi-del) 12%, transparent)',
      delBorderColor: 'var(--gi-del)',
      delDiffIndicatorColor: 'var(--gi-del)',
      markBackground: 'var(--gi-accbg)',
      markBorderColor: 'var(--gi-line2)',
    },
  },
});
