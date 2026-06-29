'use strict';

const { LanguageClient, TransportKind } = require('vscode-languageclient/node');
const vscode = require('vscode');
const path = require('path');
const os = require('os');

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

/** Apply a cached decoration list to an editor. */
function applyColorDecorations(editor, groups) {
    if (!editor || !groups) return;
    for (const { color, ranges } of groups) {
        editor.setDecorations(getOrCreateDecorationType(color), ranges);
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
    decorationCache.set(uri, groups);

    // Apply to any visible editor showing this URI.
    for (const editor of vscode.window.visibleTextEditors) {
        if (editor.document.uri.toString() === uri) {
            applyColorDecorations(editor, groups);
            console.log('[nls] applied', groups.length, 'colors to', uri);
        }
    }
}

function getBundledServerPath(extensionPath) {
    const platform = process.platform;
    const arch = process.arch;
    let platformKey;
    if (platform === 'win32') {
        platformKey = 'windows-x64';
    } else if (platform === 'darwin') {
        platformKey = arch === 'arm64' ? 'darwin-arm64' : 'darwin-x64';
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
    const modelChoice = config.get('model', 'bert-base');
    const mode = config.get('mode', 'pos');

    console.log('[nls] activating, mode =', mode);

    const dataDir = process.platform === 'win32'
        ? path.join(process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming'), 'natural-syntax-ls')
        : path.join(process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config'), 'natural-syntax-ls');

    const semanticModel = config.get('semanticModel', 'mpnet');

    let serverArgs;
    if (mode === 'semantic') {
        const prefix = semanticModel === 'mpnet' ? 'mpnet' : 'minilm';
        const embedFile = path.join(dataDir, `${prefix}.onnx`);
        const vocabFile = path.join(dataDir, `${prefix}_vocab.txt`);
        console.log('[nls] embed =', embedFile);
        serverArgs = ['--mode', 'semantic', '--model', embedFile, '--vocab', vocabFile];
    } else {
        const slug = modelChoice === 'bert-base' ? 'bert_base' : 'mobilebert';
        serverArgs = [
            '--model', path.join(dataDir, `${slug}.onnx`),
            '--vocab', path.join(dataDir, `${slug}_vocab.txt`),
        ];
    }

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

        // Reapply cached decorations when switching to an editor we've already colored.
        context.subscriptions.push(
            vscode.window.onDidChangeActiveTextEditor(editor => {
                if (!editor) return;
                const cached = decorationCache.get(editor.document.uri.toString());
                if (cached) applyColorDecorations(editor, cached);
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
