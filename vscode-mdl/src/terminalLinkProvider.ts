// SPDX-License-Identifier: Apache-2.0

import * as vscode from 'vscode';

/**
 * A terminal link that points to a Mendix element (entity, microflow, page, etc.).
 */
interface MendixTerminalLink extends vscode.TerminalLink {
	qualifiedName: string;
	elementType: string;
}

/**
 * Keyword-to-element-type mapping for inferring type from context.
 */
const keywordToType: Record<string, string> = {
	'Entity': 'entity',
	'Enumeration': 'enumeration',
	'Microflow': 'microflow',
	'Nanoflow': 'nanoflow',
	'Page': 'page',
	'Snippet': 'snippet',
	'JavaAction': 'javaaction',
	'Constant': 'constant',
	'ScheduledEvent': 'scheduledevent',
	'Queue': 'queue',
	'ODataClient': 'odataclient',
	'ODataService': 'odataservice',
};

/**
 * Infer the Mendix element type from context surrounding a qualified name on a line.
 */
function inferTypeFromLine(line: string, matchIndex: number): string {
	// Check for a keyword immediately before the qualified name
	const before = line.substring(0, matchIndex).trimEnd();
	for (const [keyword, type] of Object.entries(keywordToType)) {
		if (before.endsWith(keyword)) {
			return type;
		}
	}

	// If '(' follows the name, likely a microflow/nanoflow
	const after = line.substring(matchIndex).replace(/^[A-Z_][A-Za-z0-9_]*\.[A-Z_][A-Za-z0-9_]*/, '');
	if (after.trimStart().startsWith('(')) {
		return 'microflow';
	}

	// Scan the whole line for any known keyword
	for (const [keyword, type] of Object.entries(keywordToType)) {
		if (line.includes(keyword)) {
			return type;
		}
	}

	// Default — openElement will try fallback types
	return 'entity';
}

/**
 * TerminalLinkProvider that detects Mendix qualified names (Module.Element) in terminal
 * output and makes them clickable. Clicking opens the element's MDL description.
 */
export class MendixTerminalLinkProvider implements vscode.TerminalLinkProvider<MendixTerminalLink> {
	// Two PascalCase/upper-start identifiers separated by a dot.
	// Matches: CRM.Customer, MyModule.ACT_Create, Atlas_Core.PopupLayout
	// Excludes: fmt.Println (lowercase first char), app.mpr (lowercase)
	private readonly pattern = /\b([A-Z_][A-Za-z0-9_]*\.[A-Z_][A-Za-z0-9_]*)\b/g;

	provideTerminalLinks(
		context: vscode.TerminalLinkContext,
		_token: vscode.CancellationToken
	): MendixTerminalLink[] {
		const links: MendixTerminalLink[] = [];
		const line = context.line;

		let match: RegExpExecArray | null;
		this.pattern.lastIndex = 0;
		while ((match = this.pattern.exec(line)) !== null) {
			const qualifiedName = match[1];
			const startIndex = match.index;
			const length = qualifiedName.length;

			const elementType = inferTypeFromLine(line, startIndex);

			links.push({
				startIndex,
				length,
				tooltip: `Open ${qualifiedName} in MDL`,
				qualifiedName,
				elementType,
			});
		}

		return links;
	}

	handleTerminalLink(link: MendixTerminalLink): void {
		vscode.commands.executeCommand('mendix.openElement', link.elementType, link.qualifiedName);
	}
}
