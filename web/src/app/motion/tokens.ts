export const motionTokens = {
  transition: {
    duration: 0.2,
    ease: [0.4, 0, 0.2, 1] as const,
  },
  page: {
    initial: { opacity: 0, y: 10 },
    animate: { opacity: 1, y: 0 },
    exit: { opacity: 0, y: -10 },
  },
};
