import Image from "next/image";
import Link from "next/link";

type ArbionBrandProps = {
  className?: string;
  href?: string;
  priority?: boolean;
};

export function ArbionBrand({
  className = "",
  href,
  priority = false,
}: ArbionBrandProps) {
  const logo = (
    <Image
      src="/brand/arbion-wordmark.svg"
      width={1402}
      height={380}
      alt="Arbion"
      priority={priority}
    />
  );
  const classes = ["brand-lockup", className].filter(Boolean).join(" ");

  return href ? (
    <Link className={classes} href={href} aria-label="Arbion home">
      {logo}
    </Link>
  ) : (
    <div className={classes}>{logo}</div>
  );
}
