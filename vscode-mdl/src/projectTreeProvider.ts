// SPDX-License-Identifier: Apache-2.0

import * as vscode from 'vscode';
import * as cp from 'child_process';
import { resolvedMxcliPath } from './extension';

export interface MendixTreeNode {
	label: string;
	type: string;
	qualifiedName?: string;
	children?: MendixTreeNode[];
}

export class MendixProjectTreeProvider implements vscode.TreeDataProvider<MendixTreeNode> {
	private _onDidChangeTreeData = new vscode.EventEmitter<MendixTreeNode | undefined | void>();
	readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

	private treeData: MendixTreeNode[] = [];
	private mxcliPath: string;
	private mprPath: string | undefined;
	private pendingLoad: Promise<MendixTreeNode[]> | undefined;
	private refreshPending = false;
	private fileWatchers: vscode.FileSystemWatcher[] = [];

	constructor() {
		const config = vscode.workspace.getConfiguration('mdl');
		this.mxcliPath = resolvedMxcliPath();
		this.mprPath = this.resolveMprPath(config);
		this.setupFileWatchers();
	}

	refresh(): void {
		const config = vscode.workspace.getConfiguration('mdl');
		this.mxcliPath = resolvedMxcliPath();
		this.mprPath = this.resolveMprPath(config);
		this.treeData = [];
		// If a load is in progress, mark that a refresh is needed after it finishes
		if (this.pendingLoad) {
			this.refreshPending = true;
		}
		this.pendingLoad = undefined;
		this._onDidChangeTreeData.fire();
	}

	dispose(): void {
		for (const w of this.fileWatchers) {
			w.dispose();
		}
		this.fileWatchers = [];
	}

	private setupFileWatchers(): void {
		// Watch .mpr files (v1 format) and mprcontents/ (v2 format)
		const mprWatcher = vscode.workspace.createFileSystemWatcher('**/*.mpr');
		const mprContentsWatcher = vscode.workspace.createFileSystemWatcher('**/mprcontents/**');

		const debounceRefresh = this.createDebouncedRefresh(1000);

		mprWatcher.onDidChange(() => debounceRefresh());
		mprContentsWatcher.onDidChange(() => debounceRefresh());
		mprContentsWatcher.onDidCreate(() => debounceRefresh());
		mprContentsWatcher.onDidDelete(() => debounceRefresh());

		this.fileWatchers.push(mprWatcher, mprContentsWatcher);
	}

	private createDebouncedRefresh(delayMs: number): () => void {
		let timer: ReturnType<typeof setTimeout> | undefined;
		return () => {
			if (timer) {
				clearTimeout(timer);
			}
			timer = setTimeout(() => {
				timer = undefined;
				this.refresh();
			}, delayMs);
		};
	}

	getTreeItem(element: MendixTreeNode): vscode.TreeItem {
		const isLeaf = !element.children || element.children.length === 0;
		const collapsibleState = isLeaf
			? vscode.TreeItemCollapsibleState.None
			: vscode.TreeItemCollapsibleState.Collapsed;

		const item = new vscode.TreeItem(element.label, collapsibleState);
		item.iconPath = this.getIcon(element.type);
		item.tooltip = element.qualifiedName || element.label;

		// Add contextValue for potential future context menu differentiation
		item.contextValue = element.type;

		// System Overview: open diagram directly
		if (element.type === 'systemoverview') {
			item.command = {
				command: 'mendix.previewDiagram',
				title: 'Show System Overview',
				arguments: [element],
			};
		}

		// Openable types: leaf nodes with qualifiedName, plus non-leaf types that still have a DESCRIBE command
		const openableNonLeaf = element.type === 'projectsecurity' || element.type === 'navprofile'
			|| element.type === 'odataservice' || element.type === 'odataclient'
			|| element.type === 'publishedrestservice' || element.type === 'businesseventservice'
			|| element.type === 'databaseconnection' || element.type === 'restclient';
		if (element.type !== 'systemoverview' && (isLeaf || openableNonLeaf) && element.qualifiedName && element.type !== 'module') {
			item.command = {
				command: 'mendix.openElement',
				title: 'Open MDL Source',
				arguments: [element.type, element.qualifiedName],
			};
		}

		return item;
	}

	getChildren(element?: MendixTreeNode): Thenable<MendixTreeNode[]> {
		if (element) {
			return Promise.resolve(element.children || []);
		}

		// Root level: return cached data if available
		if (this.treeData.length > 0) {
			return Promise.resolve(this.treeData);
		}

		// Deduplicate concurrent loads: reuse pending promise if one exists
		if (this.pendingLoad) {
			return this.pendingLoad;
		}

		this.pendingLoad = this.loadProjectTree().then((data) => {
			this.pendingLoad = undefined;
			// If a refresh was requested while loading, re-trigger
			if (this.refreshPending) {
				this.refreshPending = false;
				this.treeData = [];
				this._onDidChangeTreeData.fire();
			}
			return data;
		});
		return this.pendingLoad;
	}

	private resolveMprPath(config: vscode.WorkspaceConfiguration): string | undefined {
		const configured = config.get<string>('mprPath', '');
		if (configured) {
			return configured;
		}
		return undefined; // Will auto-discover via glob
	}

	private async findMprFile(): Promise<string | undefined> {
		if (this.mprPath) {
			return this.mprPath;
		}

		// Auto-discover .mpr file in workspace
		const workspaceFolders = vscode.workspace.workspaceFolders;
		if (!workspaceFolders || workspaceFolders.length === 0) {
			return undefined;
		}

		const files = await vscode.workspace.findFiles('**/*.mpr', '**/node_modules/**', 5);
		if (files.length === 0) {
			return undefined;
		}

		// If multiple, pick the first one
		return files[0].fsPath;
	}

	private async loadProjectTree(): Promise<MendixTreeNode[]> {
		const mprFile = await this.findMprFile();
		if (!mprFile) {
			vscode.window.showWarningMessage(
				'No .mpr file found. Set mdl.mprPath in settings or open a workspace with a Mendix project.'
			);
			return [];
		}

		return new Promise<MendixTreeNode[]>((resolve) => {
			const args = ['project-tree', '-p', mprFile];
			const env = { ...process.env, MXCLI_QUIET: '1' };

			console.log(`[MDL] Running: ${this.mxcliPath} ${args.join(' ')}`);

			cp.execFile(this.mxcliPath, args, { env, maxBuffer: 10 * 1024 * 1024 }, (err, stdout, stderr) => {
				if (err) {
					const msg = stderr || err.message;
					console.error(`[MDL] Error: ${msg}`);
					vscode.window.showErrorMessage(`Failed to load project tree: ${msg}`);
					resolve([]);
					return;
				}

				try {
					this.treeData = JSON.parse(stdout) as MendixTreeNode[];
					console.log(`[MDL] Loaded ${this.treeData.length} modules`);
					// Debug: log first module's structure
					if (this.treeData.length > 0) {
						const firstModule = this.treeData.find(m => m.children && m.children.length > 0);
						if (firstModule) {
							console.log(`[MDL] Sample module "${firstModule.label}" structure:`, JSON.stringify(firstModule, null, 2).substring(0, 2000));
						}
					}
					resolve(this.treeData);
				} catch (parseErr) {
					console.error(`[MDL] Parse error: ${parseErr}`);
					vscode.window.showErrorMessage(`Failed to parse project tree JSON: ${parseErr}`);
					resolve([]);
				}
			});
		});
	}

	private getIcon(type: string): vscode.ThemeIcon {
		switch (type) {
			case 'systemoverview':
				return new vscode.ThemeIcon('graph');
			case 'module':
				return new vscode.ThemeIcon('package');
			case 'domainmodel':
				return new vscode.ThemeIcon('database');
			case 'category':
				return new vscode.ThemeIcon('folder-library');
			case 'folder':
				return new vscode.ThemeIcon('folder');
			case 'entity':
				return new vscode.ThemeIcon('symbol-class');
			case 'association':
				return new vscode.ThemeIcon('symbol-interface');
			case 'microflow':
				return new vscode.ThemeIcon('symbol-method');
			case 'nanoflow':
				return new vscode.ThemeIcon('symbol-event');
			case 'workflow':
				return new vscode.ThemeIcon('git-merge');
			case 'page':
				return new vscode.ThemeIcon('browser');
			case 'snippet':
				return new vscode.ThemeIcon('symbol-snippet');
			case 'layout':
				return new vscode.ThemeIcon('layout');
			case 'enumeration':
				return new vscode.ThemeIcon('symbol-enum');
			case 'constant':
				return new vscode.ThemeIcon('symbol-constant');
			case 'javaaction':
				return new vscode.ThemeIcon('symbol-method');
			case 'javascriptaction':
				return new vscode.ThemeIcon('symbol-event');
			case 'scheduledevent':
				return new vscode.ThemeIcon('clock');
			case 'queue':
				return new vscode.ThemeIcon('list-ordered');
			case 'buildingblock':
				return new vscode.ThemeIcon('extensions');
			case 'pagetemplate':
				return new vscode.ThemeIcon('file-symlink-file');
			case 'imagecollection':
				return new vscode.ThemeIcon('file-media');
			case 'businesseventservice':
				return new vscode.ThemeIcon('broadcast');
			case 'jsonstructure':
				return new vscode.ThemeIcon('json');
			case 'importmapping':
				return new vscode.ThemeIcon('arrow-down');
			case 'exportmapping':
				return new vscode.ThemeIcon('arrow-up');
			case 'restclient':
				return new vscode.ThemeIcon('cloud-download');
			case 'settings':
				return new vscode.ThemeIcon('gear');
			case 'settingscategory':
				return new vscode.ThemeIcon('folder-library');
			case 'settingsitem':
				return new vscode.ThemeIcon('symbol-property');
			case 'databaseconnection':
				return new vscode.ThemeIcon('plug');
			case 'publishedrestservice':
				return new vscode.ThemeIcon('globe');
			case 'restresource':
				return new vscode.ThemeIcon('symbol-class');
			case 'restoperation':
				return new vscode.ThemeIcon('symbol-method');
			case 'odataentityset':
				return new vscode.ThemeIcon('symbol-class');
			case 'odatamember':
				return new vscode.ThemeIcon('symbol-field');
			case 'bechannel':
				return new vscode.ThemeIcon('symbol-interface');
			case 'bemessage':
				return new vscode.ThemeIcon('mail');
			case 'beattribute':
				return new vscode.ThemeIcon('symbol-field');
			case 'dbquery':
				return new vscode.ThemeIcon('symbol-method');
			case 'dbqueryparam':
				return new vscode.ThemeIcon('symbol-parameter');
			case 'dbtablemapping':
				return new vscode.ThemeIcon('symbol-class');
			case 'dbcolumnmapping':
				return new vscode.ThemeIcon('symbol-field');
			case 'security':
				return new vscode.ThemeIcon('shield');
			case 'projectsecurity':
				return new vscode.ThemeIcon('shield');
			case 'modulerole':
				return new vscode.ThemeIcon('key');
			case 'userrole':
				return new vscode.ThemeIcon('person');
			case 'demouser':
				return new vscode.ThemeIcon('account');
			case 'odataclient':
				return new vscode.ThemeIcon('cloud-download');
			case 'odataservice':
				return new vscode.ThemeIcon('cloud-upload');
			case 'externalentity':
				return new vscode.ThemeIcon('remote-explorer');
			case 'navigation':
				return new vscode.ThemeIcon('compass');
			case 'navprofile':
				return new vscode.ThemeIcon('window');
			case 'navhome':
				return new vscode.ThemeIcon('home');
			case 'navlogin':
				return new vscode.ThemeIcon('lock');
			case 'navmenu':
				return new vscode.ThemeIcon('list-tree');
			case 'navmenuitem':
				return new vscode.ThemeIcon('circle-outline');
			case 'agent':
				return new vscode.ThemeIcon('robot');
			case 'aimodel':
				return new vscode.ThemeIcon('sparkle');
			case 'knowledgebase':
				return new vscode.ThemeIcon('book');
			case 'consumedmcpservice':
				return new vscode.ThemeIcon('plug');
			case 'datatransformer':
				return new vscode.ThemeIcon('symbol-function');
			default:
				return new vscode.ThemeIcon('file');
		}
	}
}
