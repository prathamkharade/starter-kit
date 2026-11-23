// Saves bytes as a user-visible file: browser download on web, app documents
// folder on mobile. Conditional import keeps web-only code out of mobile builds.
export 'save_file_io.dart' if (dart.library.js_interop) 'save_file_web.dart';
