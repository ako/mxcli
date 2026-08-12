// SPDX-License-Identifier: Apache-2.0

import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import * as os from 'os';
import {
	LanguageClient,
	LanguageClientOptions,
	ServerOptions,
} from 'vscode-languageclient/node';
import { MendixProjectTreeProvider } from './projectTreeProvider';
import { MdlFileSystemProvider } from './mdlContentProvider';
import { MdlPreviewProvider } from './previewProvider';
import { MendixTerminalLinkProvider } from './terminalLinkProvider';

// Build-time constants injected by esbuild --define (see Makefile vscode-ext target)
declare const __BUILD_TIME__: string;
declare const __GIT_COMMIT__: string;
const BUILD_TIME = typeof __BUILD_TIME__ !== 'undefined' ? __BUILD_TIME__ : 'dev';
const GIT_COMMIT = typeof __GIT_COMMIT__ !== 'undefined' ? __GIT_COMMIT__ : 'unknown';

let client: LanguageClient | undefined;
// { log: true } yields a LogOutputChannel, which is what LanguageClientOptions.outputChannel requires.
const outputChannel = vscode.window.createOutputChannel('MDL Language Server', { log: true });

const MDL_SCHEME = 'mendix-mdl';

export function activate(context: vscode.ExtensionContext) {
	const config = vscode.workspace.getConfiguration('mdl');
	const mxcliPath = resolvedMxcliPath();

	// --- Build Info ---
	outputChannel.appendLine(`MDL Extension build: ${GIT_COMMIT} (${BUILD_TIME})`);

	// --- Language Server ---
	outputChannel.appendLine(`Starting MDL Language Server: ${mxcliPath} lsp --stdio`);

	const serverOptions: ServerOptions = {
		command: mxcliPath,
		args: ['lsp', '--stdio'],
	};

	const clientOptions: LanguageClientOptions = {
		documentSelector: [
			{ scheme: 'file', language: 'mdl' },
			{ scheme: 'mendix-mdl', language: 'mdl' },
		],
		outputChannel,
		initializationOptions: {
			mprPath: config.get<string>('mprPath', ''),
			mxcliPath: config.get<string>('mxcliPath', 'mxcli'),
		},
		synchronize: {
			configurationSection: 'mdl',
		},
	};

	client = new LanguageClient(
		'mdl',
		'MDL Language Server',
		serverOptions,
		clientOptions
	);

	client.start().catch((err) => {
		const msg = `MDL Language Server failed to start: ${err.message}`;
		outputChannel.appendLine(msg);
		vscode.window.showErrorMessage(msg);
	});

	// --- Project TreeView ---
	const treeProvider = new MendixProjectTreeProvider();
	context.subscriptions.push(
		vscode.window.registerTreeDataProvider('mendixProjectTree', treeProvider)
	);
	context.subscriptions.push({ dispose: () => treeProvider.dispose() });

	// --- MDL FileSystem Provider (editable virtual documents) ---
	const fsProvider = new MdlFileSystemProvider();
	context.subscriptions.push(
		vscode.workspace.registerFileSystemProvider(MDL_SCHEME, fsProvider, {
			isCaseSensitive: true,
			isReadonly: false,
		})
	);

	// --- Terminal Link Provider (clickable Mendix artifacts in terminal) ---
	context.subscriptions.push(
		vscode.window.registerTerminalLinkProvider(new MendixTerminalLinkProvider())
	);

	// --- Commands ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.refreshProjectTree', () => {
			fsProvider.updateConfig();
			treeProvider.refresh();
		})
	);

	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.openElement', async (typeOrNode: string | any, qualifiedName?: string) => {
			// Handle both: (type, qualifiedName) from tree item click, and (node) from context menu
			let type: string;
			let name: string;
			if (typeof typeOrNode === 'object' && typeOrNode !== null) {
				// Called from context menu - first arg is the tree node
				type = typeOrNode.type;
				name = typeOrNode.qualifiedName;
			} else {
				// Called from tree item click - args are (type, qualifiedName)
				type = typeOrNode;
				name = qualifiedName!;
			}
			if (!type || !name) {
				vscode.window.showWarningMessage('Cannot open element: missing type or name.');
				return;
			}

			// Try the given type first, then fallback to other common types
			const fallbackTypes = ['entity', 'microflow', 'nanoflow', 'page', 'enumeration', 'snippet', 'constant', 'javaaction', 'javascriptaction', 'scheduledevent', 'queue', 'buildingblock', 'pagetemplate', 'imagecollection', 'businesseventservice', 'databaseconnection', 'publishedrestservice', 'workflow', 'layout', 'importmapping', 'exportmapping', 'restclient', 'jsonstructure', 'agent', 'aimodel', 'knowledgebase', 'consumedmcpservice', 'datatransformer'];
			const typesToTry = [type, ...fallbackTypes.filter(t => t !== type)];

			for (const tryType of typesToTry) {
				try {
					const uri = vscode.Uri.parse(`${MDL_SCHEME}://describe/${tryType}/${name}`);
					const doc = await vscode.workspace.openTextDocument(uri);
					await vscode.languages.setTextDocumentLanguage(doc, 'mdl');
					await vscode.window.showTextDocument(doc, { preview: true });
					return;
				} catch {
					// Describe failed for this type — try the next one
					continue;
				}
			}

			vscode.window.showWarningMessage(`Could not describe element: ${name}`);
		})
	);

	// --- Run MDL File ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.runFile', async () => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || editor.document.languageId !== 'mdl') {
				vscode.window.showWarningMessage('No MDL file is active.');
				return;
			}
			// Save the file first
			if (editor.document.isDirty) {
				await editor.document.save();
			}
			const filePath = editor.document.uri.fsPath;
			runMxcliInTerminal(['exec', filePath]);
		})
	);

	// --- Check MDL File ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.checkFile', async () => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || editor.document.languageId !== 'mdl') {
				vscode.window.showWarningMessage('No MDL file is active.');
				return;
			}
			if (editor.document.isDirty) {
				await editor.document.save();
			}
			const filePath = editor.document.uri.fsPath;
			runMxcliInTerminal(['check', filePath, '--references']);
		})
	);

	// --- Run MDL Selection ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.runSelection', async () => {
			const editor = vscode.window.activeTextEditor;
			if (!editor || editor.document.languageId !== 'mdl') {
				vscode.window.showWarningMessage('No MDL file is active.');
				return;
			}
			const selection = editor.selection;
			if (selection.isEmpty) {
				vscode.window.showWarningMessage('No text selected.');
				return;
			}
			const selectedText = editor.document.getText(selection);

			// Write selection to a temp file and execute it
			const tmpFile = path.join(os.tmpdir(), `mdl-selection-${Date.now()}.mdl`);
			fs.writeFileSync(tmpFile, selectedText, 'utf8');
			runMxcliInTerminal(['exec', tmpFile]);
		})
	);

	// --- Show Context (from tree view) ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.showContext', async (node: any) => {
			if (!node?.qualifiedName) {
				vscode.window.showWarningMessage('No element selected.');
				return;
			}
			runMxcliInTerminal(['context', node.qualifiedName, '--depth', '2']);
		})
	);

	// --- Show Impact (from tree view) ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.showImpact', async (node: any) => {
			if (!node?.qualifiedName) {
				vscode.window.showWarningMessage('No element selected.');
				return;
			}
			runMxcliInTerminal(['impact', node.qualifiedName]);
		})
	);

	// --- Show Callers (from tree view) ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.showCallers', async (node: any) => {
			if (!node?.qualifiedName) {
				vscode.window.showWarningMessage('No element selected.');
				return;
			}
			runMxcliInTerminal(['callers', node.qualifiedName]);
		})
	);

	// --- Show Callees (from tree view) ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.showCallees', async (node: any) => {
			if (!node?.qualifiedName) {
				vscode.window.showWarningMessage('No element selected.');
				return;
			}
			runMxcliInTerminal(['callees', node.qualifiedName]);
		})
	);

	// --- Show References (from tree view) ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.showReferences', async (node: any) => {
			if (!node?.qualifiedName) {
				vscode.window.showWarningMessage('No element selected.');
				return;
			}
			runMxcliInTerminal(['refs', node.qualifiedName]);
		})
	);

	// --- MDL Source Content Provider (for diagram-with-source virtual documents) ---
	const MDL_SOURCE_SCHEME = 'mendix-mdl-source';
	const sourceContentProvider = new MdlSourceContentProvider();
	context.subscriptions.push(
		vscode.workspace.registerTextDocumentContentProvider(MDL_SOURCE_SCHEME, sourceContentProvider)
	);

	// --- Show Diagram (Mermaid preview) ---
	const previewProvider = new MdlPreviewProvider(sourceContentProvider);
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.previewDiagram', async (nodeOrType?: any, qualifiedName?: string) => {
			let type: string;
			let name: string;

			if (typeof nodeOrType === 'object' && nodeOrType !== null) {
				// Called from tree context menu - first arg is the tree node
				type = nodeOrType.type;
				name = nodeOrType.qualifiedName;
			} else if (typeof nodeOrType === 'string' && qualifiedName) {
				// Called programmatically with (type, qualifiedName)
				type = nodeOrType;
				name = qualifiedName;
			} else {
				vscode.window.showWarningMessage('Cannot show diagram: no element selected.');
				return;
			}

			if (!type || !name) {
				vscode.window.showWarningMessage('Cannot show diagram: missing type or name.');
				return;
			}

			// Map tree node types to describe types
			const describeType = type === 'domainmodel' ? 'entity' : type;
			await previewProvider.showDiagram(describeType, name, type);
		})
	);
	context.subscriptions.push({ dispose: () => previewProvider.dispose() });

	// --- Show Diagram with Source (split view) ---
	context.subscriptions.push(
		vscode.commands.registerCommand('mendix.previewDiagramWithSource', async (nodeOrType?: any, qualifiedName?: string) => {
			let type: string;
			let name: string;

			if (typeof nodeOrType === 'object' && nodeOrType !== null) {
				type = nodeOrType.type;
				name = nodeOrType.qualifiedName;
			} else if (typeof nodeOrType === 'string' && qualifiedName) {
				type = nodeOrType;
				name = qualifiedName;
			} else {
				vscode.window.showWarningMessage('Cannot show diagram: no element selected.');
				return;
			}

			if (!type || !name) {
				vscode.window.showWarningMessage('Cannot show diagram: missing type or name.');
				return;
			}

			const describeType = type === 'domainmodel' ? 'entity' : type;
			await previewProvider.showDiagramWithSource(describeType, name, type);
		})
	);
}

// findMprPath returns the configured mprPath or auto-discovers one in the workspace.
async function findMprPath(): Promise<string | undefined> {
	const config = vscode.workspace.getConfiguration('mdl');
	const configured = config.get<string>('mprPath', '');
	if (configured) {
		return configured;
	}
	const files = await vscode.workspace.findFiles('**/*.mpr', '**/node_modules/**', 5);
	if (files.length > 0) {
		return files[0].fsPath;
	}
	return undefined;
}

// runMxcliInTerminal runs mxcli with the given args in a VS Code terminal.
async function runMxcliInTerminal(args: string[]) {
	const mxcliPath = resolvedMxcliPath();

	const mprPath = await findMprPath();
	if (!mprPath) {
		vscode.window.showErrorMessage('No .mpr file found. Set mdl.mprPath in settings.');
		return;
	}

	const fullArgs = ['-p', mprPath, ...args];
	const cmdLine = [mxcliPath, ...fullArgs].map(a => a.includes(' ') ? `"${a}"` : a).join(' ');

	// Reuse or create a terminal
	const termName = 'MDL';
	let terminal = vscode.window.terminals.find(t => t.name === termName);
	if (!terminal) {
		terminal = vscode.window.createTerminal(termName);
	}
	terminal.show();
	terminal.sendText(cmdLine);
}

/**
 * Resolve the configured mxcliPath, turning relative paths (./mxcli)
 * into absolute paths based on the workspace root.
 */
export function resolvedMxcliPath(): string {
	const config = vscode.workspace.getConfiguration('mdl');
	let p = config.get<string>('mxcliPath', 'mxcli');
	if (p.startsWith('./') || p.startsWith('../')) {
		const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
		if (root) {
			p = path.resolve(root, p);
		}
	}
	return p;
}

export function deactivate(): Thenable<void> | undefined {
	if (!client) {
		return undefined;
	}
	return client.stop();
}

/**
 * In-memory content provider for MDL source shown alongside ELK diagrams.
 */
export class MdlSourceContentProvider implements vscode.TextDocumentContentProvider {
	private cache = new Map<string, string>();
	private _onDidChange = new vscode.EventEmitter<vscode.Uri>();
	onDidChange = this._onDidChange.event;

	setContent(key: string, content: string): vscode.Uri {
		this.cache.set(key, content);
		const uri = vscode.Uri.parse(`mendix-mdl-source:///${key}.mdl`);
		this._onDidChange.fire(uri);
		return uri;
	}

	provideTextDocumentContent(uri: vscode.Uri): string {
		// Extract key from path: "/<key>.mdl" -> "<key>"
		let key = uri.path;
		if (key.startsWith('/')) {
			key = key.substring(1);
		}
		if (key.endsWith('.mdl')) {
			key = key.substring(0, key.length - 4);
		}
		return this.cache.get(key) || '';
	}
}
