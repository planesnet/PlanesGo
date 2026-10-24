// «/issue [title]» in the chat: opens the window to create a GitHub issue in the current project (0 tokens).
// Pure functions so they can be tested: node --test src/issueCommand.test.ts

// parseIssueCommand: the title to prefill ('' if none), or null when the text is not the command.
export function parseIssueCommand(text: string): string | null {
  const m = /^\/issue(?:\s+([\s\S]*))?$/i.exec(text.trim());
  return m ? (m[1] || '').trim() : null;
}
