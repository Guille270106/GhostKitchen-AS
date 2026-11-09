function GhostLogo({ compact = false }) {
  return (
    <span className="logo-wrap" aria-label="GhostKitchen">
      <img className="ghost-logo" src="/logo.svg" alt="" />
      {!compact && (
        <span>
          Ghost<span>Kitchen</span>
        </span>
      )}
    </span>
  );
}

export default GhostLogo;
