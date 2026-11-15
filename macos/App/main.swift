import AppKit

// phoneset Camera has no window of its own: the phoneset program on this Mac starts it, installs and removes the
// camera through it and shows what it is doing (`phoneset camera`, the menu bar). It is an app only because macOS
// installs a camera from one.
let application = NSApplication.shared
let delegate = AppDelegate()
application.delegate = delegate
application.setActivationPolicy(.accessory)
application.run()
