'use strict';

const { LanguageClient, TransportKind } = require('vscode-languageclient/node');
const vscode = require('vscode');
const path = require('path');
const os = require('os');

function getDataDir() {
    return process.platform === 'win32'
        ? path.join(process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming'), 'natural-syntax-ls')
        : path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'), 'natural-syntax-ls');
}

/** @type {LanguageClient | undefined} */
let client;

/** @type {Map<string, vscode.TextEditorDecorationType>} color hex → decoration type */
const colorDecorationTypes = new Map();

/** @type {Map<string, Array<{color:string, ranges:vscode.Range[]}>>} uri → per-color range groups */
const decorationCache = new Map();

function getOrCreateDecorationType(color) {
    if (!colorDecorationTypes.has(color)) {
        colorDecorationTypes.set(color, vscode.window.createTextEditorDecorationType({
            color,
            borderRadius: '2px',
        }));
    }
    return colorDecorationTypes.get(color);
}

/**
 * Apply a cached decoration list to an editor, clearing colors from `previous` that the new list
 * dropped; a decoration type keeps its old ranges until it is set again.
 */
function applyColorDecorations(editor, groups, previous = []) {
    if (!editor || !groups) return;
    const current = new Set(groups.map(g => g.color));
    for (const { color } of previous) {
        const dt = colorDecorationTypes.get(color);
        if (dt && !current.has(color)) editor.setDecorations(dt, []);
    }
    for (const { color, ranges } of groups) {
        editor.setDecorations(getOrCreateDecorationType(color), ranges);
    }
}

/** Dispose decoration types no cached document uses; disposing also removes them from every editor. */
function disposeUnusedDecorationTypes() {
    const used = new Set();
    for (const groups of decorationCache.values()) {
        for (const { color } of groups) used.add(color);
    }
    for (const [color, dt] of colorDecorationTypes) {
        if (!used.has(color)) {
            dt.dispose();
            colorDecorationTypes.delete(color);
        }
    }
}

/**
 * Handle a $/nls/semanticColors notification from the server.
 * @param {{ uri: string, tokens: Array<{line:number,character:number,length:number,color:string}> }} params
 */
function handleSemanticColors(params) {
    const { uri, tokens } = params;
    console.log('[nls] got semanticColors for', uri, '—', tokens.length, 'tokens');

    // Group ranges by color.
    /** @type {Map<string, vscode.Range[]>} */
    const byColor = new Map();
    for (const { line, character, length, color } of tokens) {
        if (!byColor.has(color)) byColor.set(color, []);
        byColor.get(color).push(new vscode.Range(line, character, line, character + length));
    }

    const groups = [];
    for (const [color, ranges] of byColor) {
        groups.push({ color, ranges });
    }
    const previous = decorationCache.get(uri) || [];
    decorationCache.set(uri, groups);

    // Apply to any visible editor showing this URI.
    for (const editor of vscode.window.visibleTextEditors) {
        if (editor.document.uri.toString() === uri) {
            applyColorDecorations(editor, groups, previous);
            console.log('[nls] applied', groups.length, 'colors to', uri);
        }
    }
    disposeUnusedDecorationTypes();
}

/**
 * Mirrors scripts/export_model.py's slug rule: dashes, slashes, and dots all
 * become underscores (e.g. "en_ewt.electra-base" -> "en_ewt_electra_base").
 */
function slugify(modelName) {
    return modelName.replace(/[-./]/g, '_');
}

function getBundledServerPath(extensionPath) {
    const platform = process.platform;
    let platformKey;
    if (platform === 'win32') {
        platformKey = 'windows-x64';
    } else if (platform === 'darwin') {
        platformKey = 'darwin-arm64';
    } else {
        platformKey = 'linux-x64';
    }
    const binaryName = platform === 'win32' ? 'natural-syntax-ls.exe' : 'natural-syntax-ls';
    return path.join(extensionPath, 'bin', platformKey, binaryName);
}

async function activate(context) {
    const config = vscode.workspace.getConfiguration('naturalSyntaxLs');
    const serverPath = config.get('serverPath', '') || getBundledServerPath(context.extensionPath);
    const filetypes = config.get('filetypes', ['plaintext', 'markdown']);
    const tokenMapUpdate = config.get('tokenMapUpdate', {});
    const scoreThreshold = config.get('scoreThreshold', null);
    const wiktionaryDefinitions = config.get('wiktionaryDefinitions', true);
    const semanticLightness = config.get('semanticLightness', 0.75);
    const semanticChroma = config.get('semanticChroma', 0.14);
    const mode = config.get('mode', 'pos');
    const modelName = config.get('model', 'bert-base');

    console.log('[nls] activating, mode =', mode, 'model =', modelName);

    const dataDir = getDataDir();

    // Filenames follow scripts/export_model.py's own naming convention, so no
    // per-mode setting is needed: {slug}.onnx / {slug}_vocab.txt for pos and
    // semantic modes, {slug}_dependency.onnx / {slug}_dependency_vocab.txt
    // for dependency mode (slug = modelName with -, ., / turned into _).
    const slug = slugify(modelName);
    const suffix = mode === 'dependency' ? '_dependency' : '';
    const modelFile = path.join(dataDir, `${slug}${suffix}.onnx`);
    const vocabFile = path.join(dataDir, `${slug}${suffix}_vocab.txt`);
    console.log('[nls] model =', modelFile);

    const serverArgs = ['--mode', mode, '--model', modelFile, '--vocab', vocabFile];

    const serverOptions = {
        command: serverPath,
        args: serverArgs,
        transport: TransportKind.stdio,
    };

    const documentSelector = filetypes.map(lang => ({ scheme: 'file', language: lang }));

    const initializationOptions = (() => {
        const opts = {};
        if (Object.keys(tokenMapUpdate).length > 0) opts.token_map_update = tokenMapUpdate;
        if (scoreThreshold !== null) opts.score_threshold = scoreThreshold;
        if (!wiktionaryDefinitions) opts.wiktionary_definitions = false;
        if (semanticLightness !== 0.75) opts.semantic_lightness = semanticLightness;
        if (semanticChroma !== 0.14) opts.semantic_chroma = semanticChroma;
        return Object.keys(opts).length > 0 ? opts : undefined;
    })();

    client = new LanguageClient(
        'naturalSyntaxLs',
        'Natural Syntax LS',
        serverOptions,
        { documentSelector, initializationOptions },
    );

    await client.start();
    console.log('[nls] client started');

    if (mode === 'semantic') {
        // Listen for the server's color push notification.
        client.onNotification('$/nls/semanticColors', handleSemanticColors);

        // Reapply cached decorations to editors that become visible (tab switch, split, reopen); VS Code gives them fresh, undecorated editor objects.
        context.subscriptions.push(
            vscode.window.onDidChangeVisibleTextEditors(editors => {
                for (const editor of editors) {
                    const cached = decorationCache.get(editor.document.uri.toString());
                    if (cached) applyColorDecorations(editor, cached);
                }
            })
        );

        // The server drops a closed document's state; drop its colors too so their types can be freed.
        context.subscriptions.push(
            vscode.workspace.onDidCloseTextDocument(doc => {
                if (decorationCache.delete(doc.uri.toString())) disposeUnusedDecorationTypes();
            })
        );

        context.subscriptions.push(new vscode.Disposable(() => {
            for (const [, dt] of colorDecorationTypes) dt.dispose();
            colorDecorationTypes.clear();
            decorationCache.clear();
        }));
    }
}

async function deactivate() {
    if (client) {
        await client.stop();
        client = undefined;
    }
}

module.exports = { activate, deactivate };
