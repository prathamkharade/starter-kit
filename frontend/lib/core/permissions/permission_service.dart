import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:material_ui/material_ui.dart';
import 'package:permission_handler/permission_handler.dart';

/// One place for runtime permissions. Ask in context (right before the feature
/// needs it), explain why first, and degrade gracefully when denied.
///
///   final ok = await ref.read(permissionServiceProvider).ensure(context,
///       Permission.camera, rationale: 'Needed to scan receipts.');
///
/// Remember the platform side: AndroidManifest.xml `<uses-permission>` and
/// ios/Runner/Info.plist usage strings (see docs/FRONTEND.md#permissions).
class PermissionService {
  Future<bool> ensure(BuildContext context, Permission permission, {required String rationale}) async {
    if (kIsWeb) return true; // browsers prompt on use (camera, location, notifications)
    var status = await permission.status;
    if (status.isGranted || status.isLimited) return true;

    if (!context.mounted) return false;
    final proceed = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        title: const Text('Permission needed'),
        content: Text(rationale),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: const Text('Not now')),
          FilledButton(
            onPressed: () => Navigator.pop(c, true),
            child: Text(status.isPermanentlyDenied ? 'Open settings' : 'Continue'),
          ),
        ],
      ),
    );
    if (proceed != true) return false;
    if (status.isPermanentlyDenied) {
      await openAppSettings();
      return false;
    }
    status = await permission.request();
    return status.isGranted || status.isLimited;
  }
}

final permissionServiceProvider = Provider((ref) => PermissionService());
