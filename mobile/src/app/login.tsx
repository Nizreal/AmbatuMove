import { useState } from "react";
import { Image, Pressable, View } from "react-native";
import { Button, Input, Notice, Screen, Txt, styles } from "../components/ui";
import { useStore } from "../hooks/store";
import { colors, space } from "../theme";
import { describeApiError } from "../services/ambatumove";

export default function Login() {
  const s = useStore();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [show, setShow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [createAccount, setCreateAccount] = useState(false);
  async function submit() {
    setError("");
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()))
      return setError("Masukkan alamat email yang valid.");
    if (password.length < 12)
      return setError("Kata sandi harus terdiri dari minimal 12 karakter.");
    setBusy(true);
    try {
      await s.login(email, password, createAccount);
    } catch (e) {
      setError(describeApiError(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Screen standalone>
      <View style={styles.loginBrand}>
        <Image
          source={require("../../assets/figma/brand-mark.png")}
          resizeMode="contain"
          style={{ width: 56, height: 48 }}
          accessibilityLabel="AmbatuMove"
        />
        <Txt variant="heading" color={colors.primary}>
          AmbatuMove
        </Txt>
      </View>
      <View style={{ gap: space.sm }}>
        <Txt variant="title" color={createAccount ? colors.primary : colors.text}>
          {createAccount ? "Buat akun AmbatuMove" : "Selamat datang"}
        </Txt>
        <Txt color={colors.muted}>
          Rencanakan latihan, jalani setiap gerakan, dan pantau progres Anda.
        </Txt>
      </View>
      <View style={styles.group}>
        <Input
          label="Email"
          testID="email"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          keyboardType="email-address"
          autoComplete="email"
          returnKeyType="next"
        />
        <View style={{ position: "relative" }}>
          <Input
            label="Kata sandi"
            testID="password"
            value={password}
            onChangeText={setPassword}
            secureTextEntry={!show}
            autoCapitalize="none"
            autoComplete={createAccount ? "new-password" : "current-password"}
            textContentType={createAccount ? "newPassword" : "password"}
            returnKeyType="done"
            onSubmitEditing={() => void submit()}
            style={{ paddingRight: 54 }}
          />
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={
              show ? "Sembunyikan kata sandi" : "Tampilkan kata sandi"
            }
            onPress={() => setShow(!show)}
            style={styles.passwordToggle}
          >
            <Txt variant="small" color={colors.primary}>
              {show ? "Sembunyikan" : "Lihat"}
            </Txt>
          </Pressable>
        </View>
      </View>
      {!!error && <Notice error message={error} />}
      <Button
        title={createAccount ? "Buat akun" : "Masuk"}
        testID="login"
        busy={busy}
        onPress={() => void submit()}
      />
      <Pressable
        accessibilityRole="button"
        onPress={() => {
          setCreateAccount((value) => !value);
          setError("");
        }}
        style={{ minHeight: 48, alignItems: "center", justifyContent: "center" }}
      >
        <Txt variant="label" color={colors.primary}>
          {createAccount ? "Sudah punya akun? Masuk" : "Belum punya akun? Buat akun"}
        </Txt>
      </Pressable>
    </Screen>
  );
}
