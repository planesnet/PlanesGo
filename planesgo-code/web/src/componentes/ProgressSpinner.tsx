import { useEffect, useRef } from 'react';

interface ProgressSpinnerProps {
  className?: string;
  title?: string;
  ariaLabel?: string;
}

export function spinnerAngleDegrees(timestampMs: number, periodMs = 800): number {
  if (!Number.isFinite(timestampMs) || periodMs <= 0) return 0;
  const mod = ((timestampMs % periodMs) + periodMs) % periodMs;
  return Math.round((mod / periodMs) * 360);
}

export default function ProgressSpinner({ className, title, ariaLabel }: ProgressSpinnerProps) {
  const svgRef = useRef<SVGSVGElement | null>(null);

  useEffect(() => {
    let frameId = 0;
    const updateRotation = (now: number) => {
      if (svgRef.current) {
        svgRef.current.style.transform = `rotate(${spinnerAngleDegrees(now)}deg)`;
      }
      frameId = window.requestAnimationFrame(updateRotation);
    };
    frameId = window.requestAnimationFrame(updateRotation);
    return () => window.cancelAnimationFrame(frameId);
  }, []);

  return (
    <svg
      ref={svgRef}
      className={className ? `progress-spinner ${className}` : 'progress-spinner'}
      viewBox="0 0 12 12"
      width="12"
      height="12"
      aria-hidden={ariaLabel ? undefined : true}
      aria-label={ariaLabel}
      role={ariaLabel ? 'img' : undefined}
    >
      {title ? <title>{title}</title> : null}
      <circle
        cx="6"
        cy="6"
        r="4.5"
        fill="none"
        stroke="var(--acento)"
        strokeOpacity="0.25"
        strokeWidth="2"
      />
      <path
        d="M 6 1.5 A 4.5 4.5 0 0 1 10.5 6"
        fill="none"
        stroke="var(--acento)"
        strokeWidth="2"
        strokeLinecap="round"
      />
    </svg>
  );
}
