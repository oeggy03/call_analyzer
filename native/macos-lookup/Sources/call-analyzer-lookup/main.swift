import AppKit

let application = NSApplication.shared
let delegate = LookupAgent()
application.delegate = delegate
_ = application.setActivationPolicy(.accessory)
application.run()
