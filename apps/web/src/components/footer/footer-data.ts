export interface FooterNavItem {
  index: string;
  label: string;
  href: string;
}

export interface FooterLink {
  label: string;
  href: string;
  external?: boolean;
}

/** 页脚导航：桌面端纵向排列，移动端整行可点 */
export const footerNav: FooterNavItem[] = [
  { index: "01.", label: "首页", href: "/" },
  { index: "02.", label: "文章", href: "/articles" },
  { index: "03.", label: "摄影", href: "/photos" },
  { index: "04.", label: "搜索", href: "/search" },
  { index: "05.", label: "关于", href: "/about" },
];

/** 联络邮箱 */
export const contactEmail = "hello@twilightlog.com";

/** 关注渠道 */
export const followLinks: FooterLink[] = [
  { label: "GitHub", href: "https://github.com/yisskqu/TwilightLog", external: true },
];

/** 备案信息 */
export const filings: FooterLink[] = [
  { label: "吉ICP备2026007165号-1", href: "https://beian.miit.gov.cn/", external: true },
];

/** 公安联网备案（带徽标） */
export const policeFiling: FooterLink = {
  label: "浙公网安备33011002020165号",
  href: "https://beian.mps.gov.cn/#/query/webSearch?code=33011002020165",
  external: true,
};

export const copyright = `© ${new Date().getFullYear()} TwilightLog`;
