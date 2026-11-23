import 'dart:io';
import 'dart:typed_data';

import 'package:path_provider/path_provider.dart';

/// Returns a human-readable location of the saved file.
Future<String> saveFile(Uint8List bytes, String name, String mime) async {
  final dir = await getApplicationDocumentsDirectory();
  final file = File('${dir.path}/$name');
  await file.writeAsBytes(bytes, flush: true);
  return file.path;
}
