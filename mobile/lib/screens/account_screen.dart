// Lets a manager secure their account and edit the franchise and sport identities they own.

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../data/api_client.dart';
import '../data/models.dart';
import '../data/session_controller.dart';
import 'profile_avatar.dart';

class AccountScreen extends StatefulWidget {
  const AccountScreen({
    super.key,
    required this.api,
    required this.session,
    required this.onSignOut,
    this.selectedLeague,
  });
  final ApiClient api;
  final SessionController session;
  final MyLeague? selectedLeague;
  final Future<void> Function() onSignOut;

  @override
  State<AccountScreen> createState() => _AccountScreenState();
}

class _AccountScreenState extends State<AccountScreen> {
  final email = TextEditingController();
  final emailPassword = TextEditingController();
  final code = TextEditingController();
  final oldPassword = TextEditingController();
  final newPassword = TextEditingController();
  Dynasty? dynasty;
  MyLeague? editableLeague;
  bool loadingIdentity = true;
  bool busy = false;
  bool codeSent = false;
  String? identityError;

  @override
  void initState() {
    super.initState();
    loadIdentity();
  }

  @override
  void dispose() {
    for (final controller in [
      email,
      emailPassword,
      code,
      oldPassword,
      newPassword,
    ]) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> loadIdentity() async {
    setState(() {
      loadingIdentity = true;
      identityError = null;
    });
    try {
      final franchiseId = widget.session.session?.franchise?.id;
      if (franchiseId == null || franchiseId.isEmpty) {
        if (mounted) {
          setState(() {
            dynasty = null;
            editableLeague = null;
            loadingIdentity = false;
          });
        }
        return;
      }
      final memberships = await widget.api.myLeagues();
      final owned = memberships
          .where((item) => item.ready && item.franchiseId == franchiseId)
          .toList();
      if (owned.isEmpty) {
        if (mounted) {
          setState(() {
            dynasty = null;
            editableLeague = null;
            loadingIdentity = false;
          });
        }
        return;
      }
      final current = widget.selectedLeague;
      if (current != null && !owned.any((item) => item.id == current.id)) {
        throw const ApiException(
          'Team settings are not available for this league in the app yet.',
        );
      }
      final candidate = current ?? owned.first;
      final live = await widget.api.dynasty();
      if (live.id != candidate.id ||
          !live.franchises.any((f) => f.id == franchiseId)) {
        throw const ApiException(
          'Team settings are not available for this league in the app yet.',
        );
      }
      if (mounted) {
        setState(() {
          dynasty = live;
          editableLeague = candidate;
          loadingIdentity = false;
        });
      }
    } catch (error) {
      if (mounted) {
        setState(() {
          dynasty = null;
          editableLeague = null;
          loadingIdentity = false;
          identityError = '$error';
        });
      }
    }
  }

  void notice(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  Future<void> sendEmailCode() async {
    if (email.text.trim().isEmpty || emailPassword.text.isEmpty) {
      notice('Enter the new email and your current password.');
      return;
    }
    setState(() => busy = true);
    try {
      await widget.api.beginEmailChange(emailPassword.text, email.text.trim());
      if (mounted) setState(() => codeSent = true);
      notice('Check the new address for a confirmation code.');
    } catch (error) {
      notice('$error');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> confirmEmail() async {
    if (code.text.trim().isEmpty) {
      notice('Enter the code from your email.');
      return;
    }
    setState(() => busy = true);
    try {
      await widget.api.confirmEmailChange(code.text.trim());
      await widget.session.clearLocalSession();
      notice('Email changed. Sign in with your new address.');
    } catch (error) {
      notice('$error');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> changePassword() async {
    if (oldPassword.text.isEmpty || newPassword.text.isEmpty) {
      notice('Enter your current and new passwords.');
      return;
    }
    setState(() => busy = true);
    try {
      await widget.api.changePassword(oldPassword.text, newPassword.text);
      await widget.session.clearLocalSession();
      notice('Password changed. Sign in again.');
    } catch (error) {
      notice('$error');
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> deleteAccount() async {
    List<Json> options;
    try {
      options = await widget.api.deletionOptions();
    } catch (error) {
      notice('$error');
      return;
    }
    if (!mounted) return;
    final password = TextEditingController();
    final selected = <String, String>{};
    String? errorText;
    bool deleting = false;
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (dialogContext, update) {
          final blocked = options.any(
            (option) => (option['successors'] as List<dynamic>? ?? []).isEmpty,
          );
          return AlertDialog(
            title: const Text('Delete account?'),
            content: SingleChildScrollView(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'This permanently removes your login and private manager content. League records stay, under an anonymous franchise.',
                  ),
                  const SizedBox(height: 16),
                  for (final option in options) ...[
                    Text(
                      'Transfer commissioner for this league',
                      style: Theme.of(context).textTheme.titleSmall,
                    ),
                    const SizedBox(height: 6),
                    if ((option['successors'] as List<dynamic>? ?? []).isEmpty)
                      const Text(
                        'Another manager must join this league before you can delete this account.',
                      ),
                    if ((option['successors'] as List<dynamic>? ?? [])
                        .isNotEmpty)
                      DropdownButtonFormField<String>(
                        decoration: const InputDecoration(
                          labelText: 'New commissioner',
                        ),
                        items: [
                          for (final raw
                              in option['successors'] as List<dynamic>)
                            DropdownMenuItem(
                              value: asJson(raw)['franchise_id'] as String,
                              child: Text(asJson(raw)['name'] as String),
                            ),
                        ],
                        onChanged: (value) => update(() {
                          if (value != null) {
                            selected[option['dynasty_id'] as String] = value;
                          }
                        }),
                      ),
                    const SizedBox(height: 14),
                  ],
                  TextField(
                    controller: password,
                    obscureText: true,
                    decoration: const InputDecoration(
                      labelText: 'Current password',
                    ),
                  ),
                  if (errorText != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 12),
                      child: Text(
                        errorText!,
                        style: TextStyle(
                          color: Theme.of(context).colorScheme.error,
                        ),
                      ),
                    ),
                ],
              ),
            ),
            actions: [
              TextButton(
                onPressed: deleting ? null : () => Navigator.pop(dialogContext),
                child: const Text('Cancel'),
              ),
              FilledButton(
                onPressed: deleting || blocked
                    ? null
                    : () async {
                        if (password.text.isEmpty ||
                            selected.length != options.length) {
                          update(
                            () => errorText = 'Enter your password and choose every required commissioner.',
                          );
                          return;
                        }
                        update(() {
                          deleting = true;
                          errorText = null;
                        });
                        try {
                          await widget.api.deleteAccount(
                            password.text,
                            selected,
                          );
                          await widget.session.clearLocalSession();
                          if (dialogContext.mounted) {
                            Navigator.pop(dialogContext);
                          }
                        } catch (error) {
                          if (dialogContext.mounted) {
                            update(() => errorText = '$error');
                          }
                        } finally {
                          if (dialogContext.mounted) {
                            update(() => deleting = false);
                          }
                        }
                      },
                child: const Text('Delete account'),
              ),
            ],
          );
        },
      ),
    );
    password.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final session = widget.session.session;
    if (session == null) return const SizedBox.shrink();
    final franchise = session.franchise;
    final managerName = franchise?.managerName.trim().isNotEmpty == true
        ? franchise!.managerName
        : session.email;
    final owned = dynasty?.franchises
        .where((item) => item.id == franchise?.id)
        .firstOrNull;
    final content = <Widget>[
      Row(
        children: [
          ProfileAvatar(
            name: franchise?.name ?? managerName,
            imageUrl: franchise?.imageUrl ?? '',
            size: 62,
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  managerName,
                  style: Theme.of(context).textTheme.headlineSmall,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
                Text(
                  session.email,
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
              ],
            ),
          ),
        ],
      ),
      const SizedBox(height: 28),
      _sectionTitle(context, 'Email'),
      Text(
        session.verified ? 'Verified account' : 'Email not verified',
        style: TextStyle(
          color: session.verified
              ? const Color(0xff62d27a)
              : const Color(0xffffbf69),
        ),
      ),
      if (!session.verified)
        TextButton.icon(
          onPressed: busy
              ? null
              : () async {
                  try {
                    await widget.api.resendVerification(session.email);
                    notice('Verification link sent, if needed.');
                  } catch (error) {
                    notice('$error');
                  }
                },
          icon: const Icon(Icons.mark_email_unread_outlined),
          label: const Text('Resend verification'),
        ),
      const SizedBox(height: 12),
      TextField(
        controller: email,
        keyboardType: TextInputType.emailAddress,
        autofillHints: const [AutofillHints.email],
        decoration: const InputDecoration(labelText: 'New email address'),
      ),
      const SizedBox(height: 10),
      TextField(
        controller: emailPassword,
        obscureText: true,
        decoration: const InputDecoration(labelText: 'Current password'),
      ),
      const SizedBox(height: 12),
      FilledButton.tonal(
        onPressed: busy ? null : sendEmailCode,
        child: const Text('Send confirmation code'),
      ),
      if (codeSent) ...[
        const SizedBox(height: 14),
        TextField(
          controller: code,
          keyboardType: TextInputType.number,
          maxLength: 8,
          decoration: const InputDecoration(
            labelText: 'Code sent to new email',
          ),
        ),
        FilledButton(
          onPressed: busy ? null : confirmEmail,
          child: const Text('Confirm new email'),
        ),
      ],
      const SizedBox(height: 32),
      _sectionTitle(context, 'Password'),
      TextField(
        controller: oldPassword,
        obscureText: true,
        decoration: const InputDecoration(labelText: 'Current password'),
      ),
      const SizedBox(height: 10),
      TextField(
        controller: newPassword,
        obscureText: true,
        decoration: const InputDecoration(
          labelText: 'New password',
          helperText: 'At least 8 characters',
        ),
      ),
      const SizedBox(height: 12),
      FilledButton.tonal(
        onPressed: busy ? null : changePassword,
        child: const Text('Change password'),
      ),
      const SizedBox(height: 32),
      _sectionTitle(context, 'Team identity'),
      if (loadingIdentity) const LinearProgressIndicator(),
      if (identityError != null) ...[
        Text(identityError!),
        TextButton(
          onPressed: loadIdentity,
          child: const Text('Retry team settings'),
        ),
      ],
      if (!loadingIdentity && identityError == null && dynasty == null)
        const Text(
          'Join a playable league to set your franchise and team identities.',
        ),
      if (dynasty != null && owned != null && editableLeague != null) ...[
        Text(
          'Names and images are visible throughout ${editableLeague!.name}. Sport teams use your franchise identity when left blank.',
        ),
        const SizedBox(height: 14),
        _IdentityEditor(
          key: ValueKey(
            'franchise-${owned.id}-${owned.name}-${owned.imageUrl}',
          ),
          title: 'Franchise',
          name: owned.name,
          imageUrl: owned.imageUrl,
          requiredName: true,
          onSave: (name, image) async {
            await widget.api.setFranchiseIdentity(owned.id, name, image);
            await loadIdentity();
            await widget.session.refresh();
          },
        ),
        for (final league in dynasty!.leagues) ...[
          const SizedBox(height: 14),
          _IdentityEditor(
            key: ValueKey(
              'team-${league.id}-${dynasty!.teamIdentities.where((item) => item.franchiseId == owned.id && item.leagueId == league.id).firstOrNull?.name ?? ''}',
            ),
            title: league.name,
            name:
                dynasty!.teamIdentities
                    .where(
                      (item) =>
                          item.franchiseId == owned.id &&
                          item.leagueId == league.id,
                    )
                    .firstOrNull
                    ?.name ??
                '',
            imageUrl:
                dynasty!.teamIdentities
                    .where(
                      (item) =>
                          item.franchiseId == owned.id &&
                          item.leagueId == league.id,
                    )
                    .firstOrNull
                    ?.imageUrl ??
                '',
            fallbackName: owned.name,
            fallbackImage: owned.imageUrl,
            onSave: (name, image) async {
              await widget.api.setTeamIdentity(
                owned.id,
                league.id,
                name,
                image,
              );
              await loadIdentity();
            },
          ),
        ],
      ],
      const SizedBox(height: 32),
      _sectionTitle(context, 'Account actions'),
      OutlinedButton.icon(
        onPressed: busy ? null : widget.onSignOut,
        icon: const Icon(Icons.logout),
        label: const Text('Sign out'),
      ),
      const SizedBox(height: 12),
      Text(
        'Deleting your account cannot be undone. League results remain under an anonymous franchise.',
        style: Theme.of(context).textTheme.bodySmall,
      ),
      TextButton.icon(
        onPressed: busy ? null : deleteAccount,
        icon: const Icon(Icons.delete_outline),
        label: const Text('Delete account'),
        style: TextButton.styleFrom(
          foregroundColor: Theme.of(context).colorScheme.error,
        ),
      ),
    ];
    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 24, 20, 40),
      children: [
        Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 720),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: content,
            ),
          ),
        ),
      ],
    );
  }

  Widget _sectionTitle(BuildContext context, String title) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: Text(
      title,
      style: Theme.of(context).textTheme.titleLarge
          ?.copyWith(fontWeight: FontWeight.w700),
    ),
  );
}

class _IdentityEditor extends StatefulWidget {
  const _IdentityEditor({
    super.key,
    required this.title,
    required this.name,
    required this.imageUrl,
    required this.onSave,
    this.fallbackName = '',
    this.fallbackImage = '',
    this.requiredName = false,
  });
  final String title;
  final String name;
  final String imageUrl;
  final String fallbackName;
  final String fallbackImage;
  final bool requiredName;
  final Future<void> Function(String, String) onSave;

  @override
  State<_IdentityEditor> createState() => _IdentityEditorState();
}

class _IdentityEditorState extends State<_IdentityEditor> {
  late final name = TextEditingController(text: widget.name);
  late final image = TextEditingController(
    text: widget.imageUrl.startsWith('data:') ? '' : widget.imageUrl,
  );
  late String uploadedImage = widget.imageUrl.startsWith('data:')
      ? widget.imageUrl
      : '';
  bool saving = false;
  String? error;

  @override
  void dispose() {
    name.dispose();
    image.dispose();
    super.dispose();
  }

  Future<void> pickImage() async {
    try {
      final picked = await ImagePicker().pickImage(
        source: ImageSource.gallery,
        maxWidth: 256,
        maxHeight: 256,
        imageQuality: 72,
        requestFullMetadata: false,
      );
      if (picked == null) return;
      final bytes = await picked.readAsBytes();
      final png = bytes.length >= 8 && bytes[0] == 0x89 && bytes[1] == 0x50;
      final jpeg = bytes.length >= 3 && bytes[0] == 0xff && bytes[1] == 0xd8;
      if ((!png && !jpeg) || bytes.length > 150 * 1024) {
        throw const ApiException(
          'Choose a PNG or JPEG image under 150 KB after resizing.',
        );
      }
      setState(() {
        uploadedImage =
            'data:image/${png ? 'png' : 'jpeg'};base64,${base64Encode(bytes)}';
        image.clear();
        error = null;
      });
    } catch (exception) {
      setState(() => error = '$exception');
    }
  }

  Future<void> save() async {
    final nextName = name.text.trim();
    if (widget.requiredName && nextName.isEmpty) {
      setState(() => error = 'Choose a franchise name.');
      return;
    }
    setState(() {
      saving = true;
      error = null;
    });
    try {
      await widget.onSave(
        nextName,
        uploadedImage.isNotEmpty ? uploadedImage : image.text.trim(),
      );
    } catch (exception) {
      if (mounted) setState(() => error = '$exception');
    } finally {
      if (mounted) setState(() => saving = false);
    }
  }

  @override
  Widget build(BuildContext context) => DecoratedBox(
    decoration: BoxDecoration(
      color: const Color(0xff141822),
      borderRadius: BorderRadius.circular(16),
    ),
    child: Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              ProfileAvatar(
                name: name.text.isEmpty ? widget.fallbackName : name.text,
                imageUrl: uploadedImage.isNotEmpty
                    ? uploadedImage
                    : image.text.isNotEmpty
                    ? image.text
                    : widget.fallbackImage,
                size: 52,
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Text(
                  widget.title,
                  style: Theme.of(context).textTheme.titleMedium
                      ?.copyWith(fontWeight: FontWeight.w700),
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          TextField(
            controller: name,
            maxLength: 80,
            onChanged: (_) => setState(() {}),
            decoration: InputDecoration(
              labelText: widget.requiredName ? 'Franchise name' : 'Team name',
              hintText: widget.fallbackName.isEmpty
                  ? null
                  : widget.fallbackName,
            ),
          ),
          TextField(
            controller: image,
            keyboardType: TextInputType.url,
            onChanged: (_) => setState(() => uploadedImage = ''),
            decoration: const InputDecoration(
              labelText: 'Image URL',
              hintText: 'https://example.com/logo.png',
            ),
          ),
          const SizedBox(height: 12),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              OutlinedButton.icon(
                onPressed: saving ? null : pickImage,
                icon: const Icon(Icons.photo_library_outlined),
                label: const Text('Choose image'),
              ),
              if (uploadedImage.isNotEmpty ||
                  image.text.isNotEmpty ||
                  widget.imageUrl.isNotEmpty)
                TextButton(
                  onPressed: saving
                      ? null
                      : () => setState(() {
                          uploadedImage = '';
                          image.clear();
                        }),
                  child: const Text('Remove image'),
                ),
              FilledButton(
                onPressed: saving ? null : save,
                child: Text(saving ? 'Saving…' : 'Save identity'),
              ),
            ],
          ),
          if (!widget.requiredName)
            const Padding(
              padding: EdgeInsets.only(top: 8),
              child: Text(
                'Leave either field blank to use your franchise identity.',
              ),
            ),
          if (error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
        ],
      ),
    ),
  );
}
