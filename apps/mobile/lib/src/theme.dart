import 'package:material_ui/material_ui.dart';

const brandColor = Color(0xFF36D6B0);
const dangerColor = Color(0xFFFF6B6B);

ThemeData buildAppTheme() {
  final scheme =
      ColorScheme.fromSeed(
        seedColor: brandColor,
        brightness: Brightness.dark,
      ).copyWith(
        surface: const Color(0xFF151B22),
        surfaceContainer: const Color(0xFF1B232C),
        surfaceContainerHigh: const Color(0xFF222C37),
        primary: brandColor,
        error: dangerColor,
      );
  final base = ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    brightness: Brightness.dark,
  );
  final text = base.textTheme.copyWith(
    displaySmall: base.textTheme.displaySmall?.copyWith(
      fontSize: 34,
      fontWeight: FontWeight.w700,
      letterSpacing: -0.8,
    ),
    headlineMedium: base.textTheme.headlineMedium?.copyWith(
      fontSize: 28,
      fontWeight: FontWeight.w700,
    ),
    headlineSmall: base.textTheme.headlineSmall?.copyWith(
      fontSize: 24,
      fontWeight: FontWeight.w700,
    ),
    titleLarge: base.textTheme.titleLarge?.copyWith(
      fontSize: 22,
      fontWeight: FontWeight.w700,
    ),
    titleMedium: base.textTheme.titleMedium?.copyWith(
      fontSize: 19,
      fontWeight: FontWeight.w600,
    ),
    titleSmall: base.textTheme.titleSmall?.copyWith(
      fontSize: 17,
      fontWeight: FontWeight.w600,
    ),
    bodyLarge: base.textTheme.bodyLarge?.copyWith(fontSize: 18, height: 1.5),
    bodyMedium: base.textTheme.bodyMedium?.copyWith(
      fontSize: 16.5,
      height: 1.5,
    ),
    bodySmall: base.textTheme.bodySmall?.copyWith(fontSize: 14.5, height: 1.4),
    labelLarge: base.textTheme.labelLarge?.copyWith(
      fontSize: 17,
      fontWeight: FontWeight.w600,
    ),
    labelMedium: base.textTheme.labelMedium?.copyWith(fontSize: 15),
  );
  return base.copyWith(
    scaffoldBackgroundColor: const Color(0xFF0D1218),
    textTheme: text,
    appBarTheme: const AppBarTheme(
      backgroundColor: Color(0xFF0D1218),
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      centerTitle: false,
    ),
    navigationBarTheme: NavigationBarThemeData(
      height: 76,
      backgroundColor: const Color(0xFF111820),
      indicatorColor: brandColor.withValues(alpha: 0.18),
      labelTextStyle: WidgetStatePropertyAll(
        text.labelMedium?.copyWith(fontWeight: FontWeight.w600),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: const Color(0xFF171F27),
      contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 17),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(16),
        borderSide: BorderSide.none,
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(16),
        borderSide: const BorderSide(color: Color(0xFF2A3542)),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(16),
        borderSide: const BorderSide(color: brandColor, width: 1.5),
      ),
      labelStyle: const TextStyle(fontSize: 16),
      hintStyle: const TextStyle(color: Color(0xFF8895A5), fontSize: 16),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(0, 56),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w700),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(0, 52),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        side: const BorderSide(color: Color(0xFF31404F)),
        textStyle: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
      ),
    ),
    snackBarTheme: const SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      showCloseIcon: true,
      contentTextStyle: TextStyle(fontSize: 16),
    ),
    dividerTheme: const DividerThemeData(color: Color(0xFF26313D)),
    dialogTheme: DialogThemeData(
      backgroundColor: const Color(0xFF171F27),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(24)),
      titleTextStyle: text.titleLarge,
      contentTextStyle: text.bodyMedium,
    ),
  );
}
