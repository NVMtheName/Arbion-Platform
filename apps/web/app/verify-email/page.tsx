import { ConfirmEmailForm, EmailRequestForm } from "../account-recovery";

export default async function VerifyEmailPage({
  searchParams,
}: {
  searchParams: Promise<{ sent?: string; request?: string }>;
}) {
  const { sent, request } = await searchParams;
  if (sent === "1" || request === "1") {
    return <EmailRequestForm kind="verification" initialSent={sent === "1"} />;
  }
  return <ConfirmEmailForm />;
}
