import { Features } from "./Features";
import { HowItWorks } from "./HowItWorks";
import { Numbers } from "./Numbers";
import { Quote } from "./Quote";
import { Pricing } from "./Pricing";
import { FinalCta } from "./FinalCta";

type ModalTab = "login" | "signup";

interface SectionsProps {
  onOpenModal: (tab: ModalTab, opener: HTMLButtonElement) => void;
}

export function Sections({ onOpenModal }: SectionsProps) {
  return (
    <>
      <Features />
      <HowItWorks />
      <Numbers />
      <Quote />
      <Pricing onOpenModal={onOpenModal} />
      <FinalCta onOpenModal={onOpenModal} />
    </>
  );
}
