import React, { PropsWithChildren, useState } from "react";
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  Text,
  TextInput,
  TextInputProps,
  StyleProp,
  TextStyle,
  View,
  StyleSheet,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { colors, radius, space, type } from "../theme";
export function Txt({
  children,
  variant = "body",
  color = colors.text,
  style,
}: PropsWithChildren<{
  variant?: keyof typeof type;
  color?: string;
  style?: StyleProp<TextStyle>;
}>) {
  return <Text style={[type[variant], { color }, style]}>{children}</Text>;
}
export function Screen({
  children,
  footer,
  standalone = false,
}: PropsWithChildren<{ footer?: React.ReactNode; standalone?: boolean }>) {
  const insets = useSafeAreaInsets();
  return (
    <KeyboardAvoidingView
      style={styles.fill}
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={[
          styles.screen,
          {
            paddingTop: space.xl + (standalone ? insets.top : 0),
            paddingBottom: Math.max(insets.bottom, space.xl) + space.xl,
          },
        ]}
      >
        {children}
      </ScrollView>
      {footer ? (
        <View
          style={{
            paddingHorizontal: space.xl,
            paddingTop: space.md,
            paddingBottom: Math.max(insets.bottom, space.md),
            backgroundColor: colors.background,
            borderTopWidth: 1,
            borderTopColor: colors.border,
          }}
        >
          {footer}
        </View>
      ) : null}
    </KeyboardAvoidingView>
  );
}
export function Button({
  title,
  onPress,
  secondary = false,
  disabled = false,
  busy = false,
  testID,
}: {
  title: string;
  onPress: () => void;
  secondary?: boolean;
  disabled?: boolean;
  busy?: boolean;
  testID?: string;
}) {
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityState={{ disabled: disabled || busy, busy }}
      disabled={disabled || busy}
      onPress={onPress}
      style={({ pressed }) => [
        styles.button,
        {
          backgroundColor: secondary
            ? colors.surface
            : pressed
              ? colors.pressed
              : colors.primary,
          borderColor: secondary ? colors.outline : colors.primary,
          opacity: disabled || busy ? 0.55 : 1,
        },
      ]}
    >
      {busy ? (
        <ActivityIndicator
          color={secondary ? colors.primary : colors.surface}
        />
      ) : (
        <Txt
          variant="label"
          color={secondary ? colors.primary : colors.surface}
        >
          {title}
        </Txt>
      )}
    </Pressable>
  );
}
export function Input({ label, ...props }: TextInputProps & { label: string }) {
  const [focus, setFocus] = useState(false);
  return (
    <View style={styles.group}>
      <Txt variant="label">{label}</Txt>
      <TextInput
        accessibilityLabel={label}
        placeholderTextColor={colors.muted}
        {...props}
        onFocus={() => setFocus(true)}
        onBlur={() => setFocus(false)}
        style={[
          styles.input,
          focus && { borderColor: colors.primary, borderWidth: 2 },
          props.style,
        ]}
      />
    </View>
  );
}
export function Chips<T extends string>({
  values,
  selected,
  onSelect,
  names,
}: {
  values: readonly T[];
  selected: string | string[];
  onSelect: (v: T) => void;
  names?: Record<string, string>;
}) {
  return (
    <View style={styles.wrap}>
      {values.map((v) => {
        const active = Array.isArray(selected)
          ? selected.includes(v)
          : selected === v;
        return (
          <Pressable
            key={v}
            accessibilityRole="button"
            accessibilityState={{ selected: active }}
            onPress={() => onSelect(v)}
            style={({ pressed }) => [
              styles.chip,
              {
                borderColor: active ? colors.primary : colors.outline,
                backgroundColor: active ? colors.accent : colors.surface,
                opacity: pressed ? 0.7 : 1,
              },
            ]}
          >
            <Txt color={active ? colors.primary : colors.text}>
              {active ? "✓ " : ""}
              {names?.[v] ?? v}
            </Txt>
          </Pressable>
        );
      })}
    </View>
  );
}
export function Notice({
  message,
  error = false,
}: {
  message: string;
  error?: boolean;
}) {
  return (
    <View accessibilityLiveRegion="polite" style={styles.notice}>
      <Txt color={error ? colors.error : colors.muted}>{message}</Txt>
    </View>
  );
}
export function ScreenHeader({
  title,
  subtitle,
  action,
}: {
  title: string;
  subtitle?: string;
  action?: React.ReactNode;
}) {
  return (
    <View style={styles.headerRow}>
      <View style={{ flex: 1, gap: space.xs }}>
        <Txt variant="title">{title}</Txt>
        {subtitle ? <Txt color={colors.muted}>{subtitle}</Txt> : null}
      </View>
      {action}
    </View>
  );
}

export function SectionTitle({ children }: PropsWithChildren) {
  return <Txt variant="label">{children}</Txt>;
}

export function Avatar({ label, size = 48 }: { label: string; size?: number }) {
  return (
    <View
      accessibilityLabel={`Avatar ${label}`}
      style={{
        width: size,
        height: size,
        borderRadius: size / 2,
        backgroundColor: colors.primarySoft,
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <Text
        style={[type.avatar, { color: colors.primary, fontSize: size * 0.34 }]}
      >
        {label}
      </Text>
    </View>
  );
}
export function Progress({ value, label }: { value: number; label: string }) {
  return (
    <View style={styles.group}>
      <Txt variant="small">{label}</Txt>
      <View
        accessibilityRole="progressbar"
        accessibilityValue={{ min: 0, max: 100, now: Math.round(value * 100) }}
        style={styles.track}
      >
        <View
          style={[
            styles.bar,
            { width: `${Math.min(100, Math.max(0, value * 100))}%` },
          ]}
        />
      </View>
    </View>
  );
}
export const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: colors.background },
  screen: {
    padding: space.xl,
    gap: space.xl,
    width: "100%",
    maxWidth: 720,
    alignSelf: "center",
  },
  group: { gap: space.sm },
  headerRow: { flexDirection: "row", alignItems: "center", gap: space.md },
  dashboardHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  dashboardAction: {
    minHeight: 76,
    flexDirection: "row",
    alignItems: "center",
    gap: space.md,
    padding: space.md,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  dashboardActionIcon: {
    width: 42,
    height: 42,
    borderRadius: radius.round,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.primarySoft,
  },
  profileIntro: {
    alignItems: "center",
    gap: space.sm,
    paddingVertical: space.xxl,
  },
  profileEmail: {
    flexDirection: "row",
    gap: space.md,
    alignItems: "center",
    padding: space.lg,
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: 1,
    borderColor: colors.border,
  },
  logoutButton: {
    minHeight: 52,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: space.sm,
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    borderWidth: 1,
    borderColor: colors.primary,
    padding: space.md,
  },
  loginBrand: {
    flexDirection: "row",
    alignItems: "center",
    gap: space.md,
    paddingTop: space.xxl,
    paddingBottom: space.md,
  },
  passwordToggle: {
    position: "absolute",
    right: 4,
    bottom: 2,
    width: 48,
    height: 48,
    alignItems: "center",
    justifyContent: "center",
  },
  demoBox: {
    minHeight: 68,
    flexDirection: "row",
    alignItems: "center",
    gap: space.md,
    padding: space.md,
    backgroundColor: colors.accentSoft,
    borderRadius: radius.lg,
  },
  emptyState: {
    alignItems: "center",
    gap: space.md,
    padding: space.xl,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  historyItem: {
    flexDirection: "row",
    alignItems: "center",
    gap: space.md,
    padding: space.md,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  historyActions: { flexDirection: "row", alignItems: "center", gap: 2 },
  historyAction: {
    width: 40,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: radius.md,
  },
  historyDeleteAction: { backgroundColor: "#FFF2F3" },
  historyIcon: {
    width: 42,
    height: 42,
    borderRadius: radius.round,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.primarySoft,
  },
  formSection: {
    gap: space.md,
    paddingBottom: space.md,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  planCard: {
    padding: space.md,
    gap: space.md,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  planRow: { flexDirection: "row", alignItems: "center", gap: space.md },
  alternativeRow: {
    minHeight: 96,
    flexDirection: "row",
    alignItems: "center",
    gap: space.md,
    padding: space.md,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  alternativeSelected: {
    borderColor: colors.primary,
    backgroundColor: colors.primarySoft,
  },
  activeHeader: {
    backgroundColor: colors.primary,
    borderRadius: radius.xl,
    padding: space.lg,
    gap: space.md,
  },
  activeHeaderTop: {
    flexDirection: "row",
    alignItems: "center",
    gap: space.md,
  },
  activeTrack: {
    height: 8,
    borderRadius: radius.round,
    overflow: "hidden",
    backgroundColor: "#7088DE",
  },
  activeProgress: {
    height: 8,
    borderRadius: radius.round,
    backgroundColor: colors.surface,
  },
  targetPanel: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: colors.surface,
    borderRadius: radius.lg,
    padding: space.md,
    borderWidth: 1,
    borderColor: colors.border,
  },
  caloriePanel: {
    minHeight: 72,
    flexDirection: "row",
    alignItems: "center",
    gap: space.sm,
    padding: space.md,
    backgroundColor: colors.accentSoft,
    borderLeftWidth: 3,
    borderLeftColor: colors.primary,
    borderRadius: radius.lg,
  },
  targetDivider: {
    width: 1,
    height: 42,
    backgroundColor: colors.border,
    marginHorizontal: space.md,
  },
  setList: {
    gap: space.sm,
    padding: space.md,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  manualCard: {
    padding: space.md,
    gap: space.md,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.xl,
  },
  manualFields: { gap: space.sm },
  row: {
    flexDirection: "row",
    gap: space.sm,
    alignItems: "center",
    flexWrap: "wrap",
  },
  wrap: { flexDirection: "row", flexWrap: "wrap", gap: space.sm },
  button: {
    minHeight: 50,
    padding: space.md,
    borderRadius: radius.lg,
    borderWidth: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  chip: {
    minHeight: space.touch,
    paddingHorizontal: space.lg,
    paddingVertical: space.md,
    borderWidth: 1,
    borderRadius: radius.md,
    justifyContent: "center",
  },
  input: {
    minHeight: 52,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.outline,
    borderRadius: radius.md,
    padding: space.md,
    color: colors.text,
    ...type.body,
  },
  notice: {
    padding: space.lg,
    backgroundColor: colors.surface,
    borderRadius: radius.md,
  },
  card: {
    backgroundColor: colors.surface,
    padding: space.lg,
    borderRadius: radius.xl,
    borderWidth: 1,
    borderColor: colors.border,
    gap: space.md,
  },
  hero: {
    backgroundColor: colors.accentSoft,
    padding: space.xl,
    borderRadius: radius.xl,
    gap: space.lg,
    borderWidth: 1,
    borderColor: colors.accent,
  },
  track: {
    height: 8,
    backgroundColor: colors.border,
    borderRadius: radius.sm,
    overflow: "hidden",
  },
  bar: { height: 8, backgroundColor: colors.primary },
  divider: {
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    paddingBottom: space.xl,
    gap: space.md,
  },
});
