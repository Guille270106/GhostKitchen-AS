function EmptyState({ title, text, children }) {
  return (
    <div className="empty-state">
      <img src="/logo.svg" alt="" />
      <h3>{title}</h3>
      {text && <p>{text}</p>}
      {children}
    </div>
  );
}

export default EmptyState;
