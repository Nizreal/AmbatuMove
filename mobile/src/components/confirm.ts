import { Alert as NativeAlert, AlertButton, Platform } from "react-native";
// Browser fallback keeps the optional web preview functional; Android uses native dialogs.
export const Alert = {
  alert(title: string, message: string, buttons: AlertButton[]) {
    if (Platform.OS !== "web") {
      NativeAlert.alert(title, message, buttons);
      return;
    }
    const accepted = globalThis.confirm(`${title}\n\n${message}`);
    const action = accepted
      ? buttons.find((b) => b.style !== "cancel")
      : buttons.find((b) => b.style === "cancel");
    action?.onPress?.();
  },
};
