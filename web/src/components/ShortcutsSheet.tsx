import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['h'], 'Home: latest topics'], [['u'], 'Unread'], [['i'], 'Notifications'], [['b'], 'Bookmarks'], [['e'], 'Events'], [['f'], 'Forums'], [['r'], 'Review (moderators)'], [[','], 'Settings']] },
  { title: 'Write', keys: [[['n'], 'New topic'], [['⌘', '↩'], 'Post what you wrote'], [['⌘', 'B'], 'Bold'], [['⌘', 'I'], 'Italic'], [['@'], 'Mention someone']] },
  { title: 'In a topic', keys: [[['j'], 'Next post'], [['k'], 'Previous post'], [['↵', 'R'], 'Reply'], [['l'], 'React with ❤️ to the post in view'], [['s'], 'Summary (AI)']] },
  { title: 'Find', keys: [[['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}
