'use strict';

const { LanguageClient, TransportKind } = require('vscode-languageclient/node');
const vscode = require('vscode');
const path = require('path');
const os = require('os');

/** @type {LanguageClient | undefined} */
let client;

async function activate(context) {
    const config = vscode.workspace.getConfiguration('naturalSyntaxLs');
    const binaryName = process.platform === 'win32' ? 'natural-syntax-ls.exe' : 'natural-syntax-ls';
    const cargoBin = path.join(os.homedir(), '.cargo', 'bin', binaryName);
    const serverPath = config.get('serverPath', '') || cargoBin;
    const filetypes = config.get('filetypes', ['plaintext', 'markdown']);
    const tokenMapUpdate = config.get('tokenMapUpdate', {});
    const scoreThreshold = config.get('scoreThreshold', null);
    const modelChoice = config.get('model', 'mobilebert');

    const serverDir = path.dirname(serverPath);
    const slug = modelChoice === 'bert-base' ? 'bert_base' : 'mobilebert';
    const modelFile = path.join(serverDir, `${slug}_pos.onnx`);
    const vocabFile = path.join(serverDir, `${slug}_vocab.txt`);

    const serverOptions = {
        command: serverPath,
        args: ['--model', modelFile, '--vocab', vocabFile],
        transport: TransportKind.stdio,
    };

    const documentSelector = filetypes.map((lang) => ({
        scheme: 'file',
        language: lang,
    }));

    const initializationOptions = (() => {
        const opts = {};
        if (Object.keys(tokenMapUpdate).length > 0) opts.token_map_update = tokenMapUpdate;
        if (scoreThreshold !== null) opts.score_threshold = scoreThreshold;
        return Object.keys(opts).length > 0 ? opts : undefined;
    })();

    const clientOptions = {
        documentSelector,
        initializationOptions,
    };

    client = new LanguageClient(
        'naturalSyntaxLs',
        'Natural Syntax LS',
        serverOptions,
        clientOptions,
    );

    await client.start();
}

async function deactivate() {
    if (client) {
        await client.stop();
        client = undefined;
    }
}

module.exports = { activate, deactivate };
